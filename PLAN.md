# Plan : réécriture du serveur de synchro (tâche 7.8)

## Contexte

[SYNC.md](SYNC.md) est maintenant complète : §§ 3, 4 et 4.9 pour le serveur, vecteurs vérifiés.

Ce dépôt n'a que le squelette d'un autre projet, « nexo ». La couche applicative entière (comptes à mot de passe, sessions JWT de 30 jours, SSO jamais branché) ne correspond à rien de la spec, et elle a des défauts :
- les migrations ne passent pas sur PostgreSQL ;
- GORM journalise les requêtes avec leurs valeurs ;
- il n'y a aucun test.

**But :** un serveur de référence conforme à l'API v1. On garde l'infrastructure générique, on remplace le reste, et chaque route est testée avec un client Go qui suit le § 3.

Presque tout le dépôt est non suivi par git (`git status`) : aucune donnée déployée n'est à migrer.

**Déjà fait :**
- le dépôt utilise fiber-oapi v3.3.1 et Go 1.27.2. Cette version de fiber-oapi a été écrite pour ce serveur : toutes les routes passent par elle, y compris les paquets binaires et le flux d'événements ;
- étape 1 (nettoyage et socle) : terminée, avec ses tests ;
- étape 2 (schéma et persistance) : terminée. Les tests PostgreSQL attendent `SYNC_TEST_POSTGRES_DSN` : la branche PostgreSQL du schéma et le `SELECT … FOR UPDATE` ne sont pas encore vérifiés ;
- étape 3 (secrets, limites, client de test) : terminée ; tous les vecteurs du § 3.10 passent en Go ;
- étape 4 (comptes, sessions, appareils) : terminée, avec des tests de bout en bout qui chiffrent comme le Mac ;
- étape 5 (synchro, idempotence, purges) : terminée ; deux Mac s'échangent de vrais enregistrements chiffrés dans les tests ;
- étape 6 (paquets d'extension) : terminée ; `GET /v1/server` annonce `blobs` ;
- étape 7 (flux d'événements) : terminée ; `GET /v1/server` annonce `events` et `blobs`.

## Ce qu'on garde et ce qu'on jette

**On garde :**
- la CLI urfave avec une sous-commande par paquet (`interfaces/cli/<nom>`, ajout dans [cmd/cmd.go](cmd/cmd.go)) ;
- le modèle des options (`infrastructure/config/*_flags.go` : option, puis variable d'environnement, puis YAML) ;
- le journal zerolog et son middleware HTTP, qui ne journalise ni en-têtes ni corps ;
- GORM pour SQLite et PostgreSQL ;
- les migrations goose en Go ;
- gocron ;
- Fiber et fiber-oapi v3.3.1 pour toutes les routes et la documentation ;
- l'organisation `domain` / `application` / `infrastructure` / `interfaces`, avec des interfaces `*Pers` dans `domain`.

**On supprime :**
- `domain/user.go`, `domain/session.go` et `application/{auth,session,user,ports}` ;
- `infrastructure/persistence/*` et `infrastructure/helpers/tokenutil` ;
- `infrastructure/http/auth_adapter.go`, remplacé par un `fiberoapi.TokenValidatorFunc` ;
- `interfaces/http/v1/{auth,user}` ;
- les options `session`, `registration` et `sso`, et `config.Auth` ;
- la tâche `clean_users_sessions` ;
- les deux migrations actuelles (cassées sur PostgreSQL) ;
- dans `apperrors`, les erreurs propres à nexo ;
- `helpers/shortuuid` ;
- `.gitmodules` (nexo-ui), `.gitignore copy` et les étapes « nexo-ui » de la CI ;
- `config.yaml` du dépôt, qui contient un secret. On garde `config-example.yaml` et on ignore `config.yaml`.

**Dépendances retirées :** `golang-jwt`, `gofiber/utils`, `lithammer/shortuuid` et `btcutil`. `x/crypto` reste pour `argon2`, et `google/uuid` sert aux identifiants.

## Contraintes et choix d'architecture

1. **Authentification par fiber-oapi.**
   - Configuration : `EnableAuthorization: true` et `AuthService: fiberoapi.TokenValidatorFunc(tokens.Validate)`. La lib vérifie le jeton avant de lire le corps.
   - `tokens.Validate` renvoie des erreurs sentinelles : `ErrTokenExpired`, `ErrInvalidToken`, `ErrDeviceRevoked` et `ErrAccountDeleted`.
   - L'`AuthErrorHandler` les associe aux codes du § 4.8 par `errors.Is`. Un en-tête absent donne `invalid_token`, et une erreur de configuration (500) donne `server_error`.
   - Le contexte d'authentification : `UserID` vaut l'`accountId`. Les `Claims` portent `deviceId`, l'empreinte du jeton et son expiration (le flux d'événements en a besoin).
   - Les routes sans jeton du § 4.1 ont `Security: "disabled"`. Il n'y a ni middleware maison, ni liste d'exceptions.
2. **Erreurs au format `application/problem+json`** :
   - `ProblemDetails: true` et `ValidationStatusCode: 400` ;
   - `ProblemCodes` : `validation_error`, `parse_error` et `type_error` donnent `invalid_request` ; `serialization_error` et `internal_error` donnent `server_error` ;
   - les gestionnaires renvoient un `*fiberoapi.Problem`, construit par `problem.New(code, detail)` avec le statut du § 4.8 ;
   - `UseNotFoundHandler()` pour que le 404 soit lui aussi un `Problem`.
3. **Paquets et flux d'événements, comme des routes fiber-oapi** :
   - `PUT /blobs/{id}` : entrée `[]byte` avec `body:"raw"`, `SuccessStatus: 201` et `Responses` pour le `200` ;
   - `GET /blobs/{id}` : sortie `*fiberoapi.RawResponse` ;
   - `GET /sync/events` : sortie `EventStream` qui implémente `Responder`, avec `ResponseContentType: "text/event-stream"` ;
   - les réponses 204 passent par `SuccessStatus: 204` ;
   - limite de débit sur une route : `OpenAPIOptions.Middlewares`, lancés après l'authentification, donc capables de lire le compte.

   Toutes les routes apparaissent dans l'OpenAPI générée.
4. **Réglages HTTP à changer dans [infrastructure/http/http.go](infrastructure/http/http.go)** :
   - `BodyLimit` de 10 Mo, plus petit qu'un paquet de 21 Mio : passer à `maxBlobBytes` + 1 Mio.
   - Un middleware du groupe `/api/v1` impose `maxRequestBytes` (`request_too_large`) partout, sauf sur `PUT /blobs/{id}`. Cette route impose `maxBlobBytes` (`blob_too_large`) par ses `Middlewares`.
   - `compress` met le flux SSE en tampon : l'exclure (`Next`) pour `/sync/events` et `/blobs`.
   - `WriteTimeout` de 30 s couperait le flux : le passer à 0, et le garder via `ReadTimeout` et `IdleTimeout`. À vérifier par un test plus long que l'ancien délai.
   - CORS est inutile pour un client natif : on le retire.
   - Ajouter `TrustProxy` et l'en-tête d'origine (option `http.trusted_proxies`), pour la limite de débit par adresse IP derrière nginx.
5. **Journaux de GORM** : le journal actuel écrit le SQL avec ses valeurs. Ajouter `ParamsFilter` (paramètres retirés) au [GormLogger](infrastructure/logger/zerolog/gorm_adapter.go), et le niveau `Warn` par défaut.
6. **SQLite** : compléter le DSN avec `_txlock=immediate`, `_busy_timeout=5000`, `_journal_mode=WAL` et `_foreign_keys=on`, dans [infrastructure/database/gorm.go](infrastructure/database/gorm.go). `_txlock=immediate` donne le `BEGIN IMMEDIATE` du § 4.9.
7. **Arrêt propre**, absent aujourd'hui. Sur SIGTERM :
   - fermer les flux du hub ;
   - `ShutdownWithTimeout` de Fiber ;
   - arrêter gocron ;
   - fermer la base.
8. **Migrations** : elles sont appliquées par la commande `migration`, jamais par le serveur (`start.sh` lance `migration` puis `server`). Au démarrage, le serveur vérifie que le schéma est à jour, et refuse de démarrer sinon, en disant de lancer `migration`. Le `Dockerfile` lance `./app server` ; `docker-compose.yaml` lancera `migration` avant lui (étape 8).

## Schéma (une migration, `files/<horodatage>_sync_schema.go`, deux dialectes)

Pas de `CONCURRENTLY`, puisque la migration tourne dans une transaction. Binaire en `BLOB` (SQLite) ou `BYTEA` (PostgreSQL).

| Table | Colonnes principales |
| --- | --- |
| `server_state` (une ligne) | `epoch` UUID, `prelogin_secret` (32 octets), `deleted_max_version` |
| `accounts` | `id`, `email` (unique, normalisé), `kdf_algorithm`, `kdf_iterations`, `kdf_salt`, `auth_hash` (PHC), `wrapped_account_key`, `recovery_auth_hash`, `recovery_wrapped_account_key`, `key_version`, `last_version`, `max_purged_version`, `usage_bytes`, `created_at`, `updated_at` |
| `devices` | clé (`account_id`, `id`), `name`, `created_at`, `last_seen_at`, `revoked_at` |
| `tokens` | `hash` (SHA-256), `account_id`, `device_id`, `kind` (`access`, `refresh`, `recovery`), `expires_at`, `used_at`, `revoked_at`, `created_at` |
| `deleted_account_tokens` | `hash`, `expires_at` (60 jours) |
| `records` | clé (`account_id`, `id`), `version`, `deleted`, `key_version`, `payload`, `size`, `updated_at` ; index (`account_id`, `version`) |
| `blobs` | clé (`account_id`, `id`), `size`, `sha256` du chiffré, `created_at` ; fichier dans `storage.data_dir/blobs/<compte>/<id>` |
| `idempotency_keys` | clé (`account_id`, `device_id`, `key`), `body_hash`, `response`, `created_at` |
| `invitations` | `id`, `code_hash`, `note`, `created_at`, `expires_at`, `used_at`, `revoked_at` |

**Jetons et révocation :**
- Une révocation marque `revoked_at` sur les jetons et sur l'appareil. Une reconnexion réactive l'appareil sans réactiver les anciens jetons.
- Un jeton d'accès expiré reste un jour en base, pour répondre `token_expired` plutôt que `invalid_token`.
- Un jeton de renouvellement utilisé reste jusqu'à son expiration, pour détecter sa réutilisation et révoquer l'appareil.

## Organisation du code (nouveaux paquets)

- **`domain/`** : `account.go`, `device.go`, `token.go`, `record.go`, `blob.go`, `invitation.go`, `idempotency.go`, `server_state.go`. Chacun avec ses entités et son interface `*Pers`.
- **`application/` :**
  - `account` : `prelogin`, `create`, `login`, `refresh`, `logout`, `recover_start`, `recover_finish`, `get`, `password`, `recovery`, `email`, `delete`, la fabrique de `Session`, et les appareils (`list`, `rename`, `revoke`), qui partagent la révocation des jetons ;
  - `records` (et non `sync`, qui masquerait le paquet standard) : `changes`, `push`, `purge` ;
  - `blob` : `put`, `get`, `delete` ;
  - `invitation` : `create`, `list`, `revoke`, `consume`.

  Même forme que l'existant : une structure avec `Config`, `Logger` et ses `Pers`, et une méthode par cas d'usage, avec ses DTO.
- **`infrastructure/` :**
  - `persistence/` : dépôts GORM. La transaction voyage dans le `context` : chaque dépôt l'utilise quand elle est là. `Transactor.InTx(ctx, fn)` ouvre une transaction, et `Transactor.WithAccountLock(ctx, accountID, fn)` une transaction qui verrouille le compte : `SELECT … FOR UPDATE` sous PostgreSQL, transaction immédiate sous SQLite.
  - `secrets/` :
    - Argon2id en chaîne PHC, avec relèvement des paramètres à la connexion, un sémaphore pour limiter les calculs simultanés (64 Mio chacun) et un hachage factice pour les comptes inconnus ;
    - jetons de 32 octets aléatoires en base64url, avec empreinte SHA-256 ;
    - sel factice HMAC-SHA256 du prelogin ;
    - comparaisons en temps constant ;
    - normalisation Crockford (invitations).
  - `blobstore/` : système de fichiers. Fichier temporaire, vérification, puis `rename` sous le verrou.
  - `epoch/` : le repère hors de la base (`data_dir/epoch-marker.json`) et la détection au démarrage. Il compare au maximum de `accounts.last_version` et de `server_state.deleted_max_version`. Le second sert à ce que la suppression d'un compte ne déclenche pas de fausse restauration.
  - `ratelimit/` : en mémoire, à fenêtre glissante.
    - Routes qui vérifient une clé : 10 essais par quart d'heure par (adresse IP, compte), et 100 par heure par compte.
    - Inscription et prelogin : une limite par adresse IP.
  - `events/` : un hub par compte. Abonnés (appareil, empreinte du jeton, expiration) et notification fusionnée sur un canal de taille 1. `CloseDevice`, `CloseAccount` et `CloseOthers` servent aux révocations.
- **`interfaces/http/` :**
  - `problem/` : `New(code, detail)`, qui construit un `*fiberoapi.Problem` avec le statut du § 4.8, et l'`AuthErrorHandler` ;
  - `middleware/` : tailles et `Matcha-Client` (option `http.minimum_client_version`, `426` quand elle est fixée) ;
  - `v1/{server,auth,accounts,account,devices,sync,blobs,events}`.
- **`interfaces/cli/admin/`** :
  - `invitations create [--expires 168h] [--note]` : affiche le code une seule fois ;
  - `invitations list` ;
  - `invitations revoke <id>` ;
  - `rotate-epoch`.
- **`infrastructure/jobs/`** : purge des jetons expirés, des clés d'idempotence de plus de 24 h, des empreintes `deleted_account_tokens` de plus de 60 jours, des invitations expirées et des fichiers temporaires. Les suppressions de plus de 180 jours sont purgées aussi, en mettant à jour `accounts.max_purged_version`.

**Nouvelles options :**
- `registration.mode` : `open`, `invitation` ou `closed`, `invitation` par défaut ;
- `sync.max_payload_bytes`, `sync.max_changes_per_push`, `sync.max_request_bytes`, `sync.max_blob_bytes` et `sync.max_account_bytes` (0 veut dire `null`) ;
- `kdf.minimum_iterations` et `kdf.maximum_iterations`, bornés à 600 000 et 10 000 000 ;
- `argon2.memory`, `argon2.time` et `argon2.threads` (64 Mio, 3, 4) ;
- `storage.data_dir` ;
- `http.trusted_proxies` et `http.minimum_client_version`.

## Points délicats de la logique (règles du § 4.9)

- **`push`** : tout se fait dans `WithAccountLock`.
  - Idempotence : la même clé avec la même empreinte de corps renvoie la réponse gardée. Avec un corps différent, c'est `400 invalid_request`. Sous le verrou, une requête concurrente avec la même clé attend naturellement la première.
  - Plus de `maxChangesPerPush` changements : `413 request_too_large`.
  - Vérifications de chaque changement :
    - `payload` en base 64 ;
    - au-delà de `maxPayloadBytes` décodés : `payload_too_large` ;
    - format : premier octet `0x01` et `(taille − 29)` multiple de 256, sinon `invalid_payload` ;
    - `keyVersion` égal à celui du compte, sinon `invalid_payload`.
  - Application, selon `baseVersion` :
    - égal à la version actuelle, ou `null` pour un identifiant inconnu : le changement est appliqué ;
    - non nul pour un identifiant inconnu : `conflict` avec `current: null` ;
    - sinon : `conflict` avec l'enregistrement actuel.
  - Version : max(`last_version` + 1, heure en µs).
  - Quota : il n'est vérifié que pour une écriture qui n'est pas une suppression.
  - Après la validation : mettre à jour le repère d'époque, puis `hub.Publish(account, device, version)`.
- **`changes`** : curseur opaque, en base64url de (époque, version). Il répond `410` si l'époque diffère, ou si la version est plus petite que `max_purged_version`. Sans `since`, pas de contrôle. La requête lit `limit + 1` lignes pour calculer `hasMore`.
- **Flux d'événements :**
  - en-têtes du § 4.9, `: ping` toutes les 30 s, `event: changes` ;
  - à l'expiration du jeton : `event: expired`, puis fermeture ;
  - à une révocation : fermeture immédiate, sans `expired`.
- **Temps de réponse constant** :
  - `prelogin` calcule toujours le sel factice ;
  - `login` et `recover/start` font toujours une vérification Argon2id (factice si le compte n'existe pas), avec la même erreur.
- **Suppression du compte**, sous le verrou :
  1. copier les empreintes des jetons dans `deleted_account_tokens` ;
  2. relever `server_state.deleted_max_version` ;
  3. tout effacer.

  Après la validation : supprimer le dossier des paquets et appeler `hub.CloseAccount`.
- **Changement de mot de passe et récupération** : révoquer les autres appareils, puis `hub.CloseOthers`.

## Tests

Le dépôt n'en a aucun : on les crée.

- **`internal/testclient`** : client Go qui suit le § 3, avec `crypto/pbkdf2`, `crypto/hkdf`, AES-GCM, le complément et les identifiants HMAC.
  - `TestSpecVectors` vérifie toutes les valeurs du § 3.10.
  - Le client sert aussi à tous les tests de bout en bout.
- **Tests de bout en bout** : vrai serveur sur `127.0.0.1:0`, SQLite dans `t.TempDir()`. Scénarios :
  1. inscription par invitation, `prelogin` d'un compte connu et d'un inconnu (même forme, sel stable) ;
  2. `login`, puis `refresh` avec rotation ; une réutilisation donne `device_revoked` ;
  3. `push`, `changes` et conflits, dont `current: null` ;
  4. idempotence : rejeu, et corps différent qui donne `400` ;
  5. quota plein, mais suppression acceptée ;
  6. `cursor_expired`, après une purge et après `rotate-epoch` ;
  7. paquets : `201`, `200`, `409`, `413` et `404` ;
  8. flux d'événements : `changes`, `expired`, et fermeture à la révocation ;
  9. changement de mot de passe, récupération, changement d'adresse e-mail ;
  10. suppression du compte, puis `account_deleted` ;
  11. `429` ;
  12. `Content-Type` en `application/problem+json` ;
  13. détection de restauration : repère plus grand que la base ;
  14. requêtes sans jeton : `401` avant toute validation du corps.
- **Concurrence** : des envois parallèles sur un même compte, et un lecteur qui pagine en même temps. Les versions doivent être strictement croissantes, sans trou vu par le lecteur. À lancer avec `-race`.
- **PostgreSQL** : la même suite si `SYNC_TEST_POSTGRES_DSN` est défini. En CI, avec un service postgres.

## Ordre de travail

Chaque étape compile et a ses tests.

1. **Nettoyage et socle** : suppressions, nouvelles options, pragmas SQLite, filtre GORM, réglages HTTP (taille, `compress`, CORS, proxy), configuration de fiber-oapi (`ProblemDetails`, `ProblemCodes`, 404) et paquet `problem`, arrêt propre, vérification du schéma au démarrage.
2. **Schéma et persistance** : `WithAccountLock`, versions, repère d'époque.
3. **Secrets et limites** : `secrets`, `ratelimit`, `internal/testclient` et `TestSpecVectors`.
4. **Comptes** : `server`, `prelogin`, `accounts`, `login`, `refresh`, `logout`, `recover/*`, `account/*`, `devices`, plus `tokens.Validate` et l'`AuthErrorHandler`.
5. **Synchro** : `sync/changes`, `sync/push` et l'idempotence, plus les tâches de purge.
6. **Paquets** : `blobs`.
7. **Événements** : le hub et le flux SSE, branchés sur les révocations.
8. **Administration et livraison** :
   - CLI d'administration ;
   - `gen-oapi` (nouvelle signature de `http.Configure`) ;
   - `Dockerfile`, `docker-compose.yaml` (service serveur et volume `data_dir`) ;
   - `Makefile` (`test`, `test-postgres`, `vet`) ;
   - CI : tests SQLite et PostgreSQL, sans les étapes nexo-ui, et `nightly.yaml` réparé ;
   - README : installation, certificats, nginx et sauvegardes, avec le repère exclu.

## Choix faits par défaut (à contester si besoin)

- **Une seule instance** : la limite de débit et le hub d'événements sont en mémoire. Un serveur public sur plusieurs instances demandera PostgreSQL `LISTEN/NOTIFY` et une limite partagée.
- **Paquets** sur le système de fichiers (`data_dir`), pas en base.
- **`GET /v1/devices`** ne liste que les appareils actifs.

## Vérification

- `go vet ./...`, puis `go test -race ./...` avec SQLite.
- La même suite avec `SYNC_TEST_POSTGRES_DSN` sur le PostgreSQL de `docker-compose.yaml`.
- Lancer `go run ./cmd server` avec un `config-example.yaml` à jour, puis faire avec `curl` :
  - `GET /api/v1/server` ;
  - créer une invitation avec la CLI ;
  - lancer le scénario complet de `internal/testclient`.
- Vérifier les journaux en niveau `debug` : aucune valeur SQL, aucun jeton, aucun `payload`.
