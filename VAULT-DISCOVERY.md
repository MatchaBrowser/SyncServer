# VAULT-DISCOVERY.md — un coffre-fort chiffré dans la synchro

Discovery du 2026-11-07, révisée le même jour avec les premières décisions (§ 10). La spécification qui en découle est [VAULT.md](VAULT.md) ; elle fait foi là où elle s'écarte d'ici (son § 13 dit où et pourquoi). Ce document pose le périmètre, ce qui existe, ce qui manque, l'état de l'art, les choix de conception et les questions encore ouvertes.

**Le sujet :** un gestionnaire de secrets à la 1Password (mots de passe, codes à usage unique, clés SSH, fichiers, cartes, identités…). Il s'appuie sur le compte et le serveur de synchro, et reste dans la veine de [SYNC.md](SYNC.md) : le serveur ne sait rien, et le modèle de menace va jusqu'au **serveur malveillant**.

---

## 0. En bref

**Décidé (2026-11-07) :**
- le coffre utilise **le même compte que la synchro** ;
- il est **intégré à Matcha** ; un client annexe (SSH ou autre) viendra plus tard ;
- **le partage viendra plus tard** ;
- **on reste sur ce que Matcha a déjà** : macOS 15.4 au minimum, CryptoKit, CommonCrypto, Security et LocalAuthentication, sans bibliothèque de crypto tierce et sans noyau commun en Rust ;
- **aucune compatibilité avec l'API Bitwarden** ;
- **le post-quantique est obligatoire.** La v1 l'est déjà, sous macOS 15, car toute sa crypto est symétrique ; seul le partage demandera X-Wing (§ 5.8) ;
- **une Secret Key pour tout le compte**, synchro comprise. Elle entre dans SYNC.md avant la mise en production, donc sans migration (§ 5.2).

**Ce que ça donne pour la v1 :**
1. **Le serveur change à peine.** Les éléments du coffre sont des enregistrements comme les autres : un nouveau type, caché dans le contenu chiffré, dans l'espace actuel du compte. Les fichiers sont des suites de paquets (§ 4.7 de SYNC.md). Il ne faut côté serveur que deux enveloppes de clé de plus et un `push` atomique (§ 6.3).
2. **Deux couches de chiffrement** (§ 5.4) :
   - la couche externe est celle d'aujourd'hui (`dataKey`) : le moteur de synchro tourne coffre verrouillé, en tâche de fond ;
   - la couche interne demande la clé du coffre, qui n'est jamais au repos en clair et n'existe en mémoire qu'une fois le coffre déverrouillé (mot de passe ou Touch ID).

   Le cache local chiffré vient tout seul.
3. **Une Secret Key**, comme chez 1Password (§ 5.2). Avec PBKDF2, qu'on garde faute d'Argon2id dans CryptoKit, c'est elle qui empêche le serveur, ou une fuite de la base, de deviner le mot de passe. C'est aussi elle qui rend inutile l'enregistrement du trafic TLS (§ 5.8). **Elle vaut pour tout le compte**, et entre directement dans le protocole de la synchro, qui n'est pas encore en production : ni migration, ni version de compte (D8, D9).
4. **Un manifeste du coffre authentifié par la clé du coffre** (§ 5.6). Il empêche le serveur, ou du code qui n'aurait que `dataKey`, de cacher, de supprimer ou de remettre à une ancienne version un élément sans être vu.
5. **Un modèle d'élément aligné sur le format d'échange de la FIDO Alliance (CXF)** (§ 7) : `ssh-key`, `api-key`, `file`, `totp`, `wifi`… y sont déjà, avec un mécanisme d'extensions.
6. **Le remplissage dans l'interface native de Matcha**, jamais dans la page, avec la liste publique des suffixes complète (§ 8).

---

## 1. Périmètre

### 1.1 Données

| Catégorie | Contenu | Remarques |
| --- | --- | --- |
| Identifiant | nom d'utilisateur, mot de passe, adresses (portée), code TOTP, champs personnalisés, historique des mots de passe | le cœur ; remplissage au § 8 |
| Code à usage unique | secret TOTP, algorithme, chiffres, période | le ranger avec le mot de passe réduit la 2FA à un facteur : le dire dans l'interface |
| Note sécurisée | texte | |
| Carte bancaire | numéro, titulaire, expiration, cryptogramme, code PIN | |
| Identité, adresse | nom, adresses, téléphone, e-mail | remplissage des formulaires |
| Documents d'identité | passeport, permis, carte d'identité | |
| Compte bancaire | IBAN, BIC, titulaire | |
| Clé SSH | clé privée, clé publique, empreinte, commentaire | v1 : stockage, génération, copie de la clé publique ; l'agent SSH viendra avec le client annexe (§ 9) |
| Clé d'API, jeton | valeur, hôte, expiration | |
| Serveur, base de données | hôte, port, utilisateur, mot de passe, base | |
| Wi-Fi | SSID, mot de passe, sécurité | |
| Licence logicielle | clé, propriétaire, version | |
| Portefeuille crypto | phrase de récupération (BIP-39), adresses | mot de passe redemandé pour l'afficher |
| Codes de secours | codes 2FA à usage unique | |
| Fichier, document | fichier chiffré, pièce jointe ou élément à part entière | § 6.2 |
| Personnalisé | sections et champs typés | tout ce qui précède se ramène à ce modèle (§ 7) |
| Passkey | clé privée, `rpId`, identifiant de l'utilisateur | plus tard : il faut être fournisseur de passkeys (§ 9) |

### 1.2 Clients

- **v1 : Matcha seul.** Matcha n'a aujourd'hui aucune fonction de mots de passe : l'interface, le stockage du coffre et le remplissage sont à écrire. Seule la base de la synchro existe (§ 2.1). Tout doit marcher sous **macOS 15**, comme Matcha (cible actuelle : 15.4).
- **Plus tard** (§ 9) : un client annexe (agent SSH, fournisseur AutoFill de macOS pour les autres navigateurs et les apps, CLI), puis peut-être une extension de navigateur.
- **Pas de « coffre web » servi par le serveur.** Une page dont le serveur fournit le JavaScript lui rend tout.

---

## 2. Ce qu'on a, et ce qui manque

### 2.1 Ce qui se réutilise

**Côté serveur (ce dépôt) :**

| Brique | Pour le coffre |
| --- | --- |
| Comptes, appareils, jetons, invitations, limite de débit, audit, console | tels quels ; deux enveloppes en plus (§ 6.3) |
| Moteur d'enregistrements : versions, curseur avec époque, `baseVersion`, idempotence, pagination par taille, purge | tel quel ; `push` atomique en plus (§ 5.6) |
| Paquets binaires (`/v1/blobs`) | tels quels : un fichier du coffre est une suite de paquets (§ 6.2) |
| Flux d'événements | tel quel |
| Client de test Go (`internal/testclient`) | étendu à la crypto du coffre : c'est l'implémentation indépendante qui vérifie les vecteurs, comme aujourd'hui |

**Côté Matcha** (`Matcha/Services/Sync/`) :

| Code existant | Pour le coffre |
| --- | --- |
| `SyncCrypto` : `pbkdf2`, `subkey` (HKDF), `seal` et `open` (AES-GCM), `pad`, `randomBytes`, chiffrement des paquets | toute la crypto symétrique de la v1 |
| `SyncAccountKeys` : `passwordKeys`, `wrap`, `unwrap`, `creation` | étendu à la Secret Key et à la clé racine du coffre (§ 5.2, § 5.3) |
| `SyncKeychain` | nouveaux éléments : Secret Key, clé du coffre enveloppée pour Touch ID |
| `SyncEngine`, `SyncFieldMerge`, `SyncNoteConflict` | les règles de fusion et de copie en cas de conflit servent de base à « aucun secret n'est perdu » (§ 6.1) |
| `SyncRecordType`, `SyncFields`, `SyncIdentifiers` | nouveaux types `vaultKey`, `vaultItem`, `vaultManifest` |
| `SyncAPIClient` (paquets), `SyncRecoveryKey`, `SyncPasswordStrength` | fichiers, kit de secours, jauge du générateur |

### 2.2 Ce qui manque, et la réponse de la v1

| Manque | Réponse |
| --- | --- |
| `accountKey` est toujours disponible (trousseau, sans verrou, § 3.8) | une clé racine du coffre distincte, jamais au repos en clair (§ 5.3) |
| Les données locales sont en clair (SwiftData) | la couche interne reste chiffrée sur le disque (§ 5.4) |
| Le serveur peut deviner le mot de passe hors ligne (§ 3.9) | Secret Key (§ 5.2) |
| « Le plus récent gagne » peut perdre un mot de passe | « aucun secret n'est perdu » (§ 6.1) |
| Le serveur peut cacher des enregistrements (§ 3.9, § 8) | manifeste du coffre (§ 5.6) |
| Fichiers limités à 21 Mio | suite de paquets (§ 6.2) |
| `SiteDomain` regroupe `alice.github.io` et `bob.github.io` | liste publique des suffixes complète pour le remplissage (§ 8) |
| Un seul espace par compte, pas de clés publiques | suffit pour un coffre personnel ; à revoir avec le partage (§ 9) |

---

## 3. État de l'art

### 3.1 Bitwarden (serveur en C#) et Vaultwarden (Rust)

Vaultwarden réimplémente l'API de Bitwarden en Rust (Rocket, Diesel, SQLite, MySQL ou PostgreSQL). Ses sources ont été lues pour cette discovery.

- **Compte** (`users`) :
  - le client dérive la clé maître par PBKDF2 (600 000 itérations par défaut) ou Argon2id, avec **l'adresse e-mail pour sel** (`master_password_salt`) ;
  - le serveur reçoit un hachage du mot de passe maître et le rehache (PBKDF2, 600 000 itérations) ;
  - il garde `akey` (la clé de l'utilisateur, enveloppée par la clé maître), une paire RSA-2048, les paramètres KDF, et `security_stamp`, qui invalide tous les jetons d'un coup.
- **Éléments** (`ciphers`) :
  - `atype` est en clair : 1 identifiant, 2 note, 3 carte, 4 identité, 5 clé SSH, 6 compte bancaire, 7 permis, 8 passeport (les types 6 à 8 sont récents, Bitwarden v2026.7) ;
  - `name`, `notes`, `fields`, `data` et `password_history` sont **chiffrés champ par champ** (EncString, AES-256-CBC et HMAC-SHA256) ;
  - restent en clair : le type, le dossier, l'organisation, les collections, le favori, la corbeille, les dates, et la structure (nombre d'adresses et de champs, longueur de chaque champ) ;
  - une clé par élément (`key`) a été ajoutée récemment.
- **Pièces jointes :** une clé par fichier, le nom chiffré, la taille en clair.
- **Synchro :** `GET /api/sync` renvoie **tout le coffre** à chaque fois. Les notifications passent par WebSocket (SignalR), et il n'y a pas de lecture incrémentale.
- **Connexion :** OAuth2 avec mot de passe, JWT, 2FA (TOTP, e-mail, WebAuthn, Duo, YubiKey), connexion par un autre appareil.
- **Partage :**
  - les organisations ont une clé enveloppée par la clé publique RSA de chaque membre ;
  - **récupération du compte par l'administrateur** : c'est un séquestre ;
  - accès d'urgence ; *Sends*.
- **Crypto « v2 »** (SDK Rust de Bitwarden, 2025-2026) : clé de signature, clé publique signée, « état de sécurité » signé, au format COSE. Vaultwarden ne la gère pas encore.

**À retenir :** le modèle est riche en fonctions, mais le serveur en voit beaucoup trop, et la crypto champ par champ, sans lien avec son élément, a donné l'essentiel des attaques du § 3.3.

### 3.2 1Password

- **Deux secrets (2SKD) :** la clé de déverrouillage dépend du mot de passe **et** d'une Secret Key de 128 bits, tirée sur l'appareil, jamais envoyée, et imprimée dans l'Emergency Kit. Les données du serveur ne permettent aucune attaque par dictionnaire.
- **SRP** (PAKE) : le serveur ne reçoit jamais d'équivalent du mot de passe.
- **Hiérarchie des clés :** un trousseau personnel, une clé par coffre enveloppée pour chaque membre, et chaque élément en deux parties chiffrées : *overview* (titre, adresses, tags : pour la liste) et *details* (les secrets).
- **Fonctions :** agent SSH (socket dans le conteneur de groupe de l'app, Touch ID à l'usage), CLI `op`, Watchtower (HIBP en k-anonymat), historique des éléments, passkeys, partage par lien.
- **Limite admise :** les clés publiques des membres ne sont pas authentifiées.

### 3.3 Les leçons de l'ETH Zurich (USENIX Security 2026)

*Zero Knowledge (About) Encryption* (Scarlata, Torrisi, Backendal, Paterson) recense **27 attaques par un serveur malveillant** :

| Catégorie | Cause | Bitwarden | LastPass | Dashlane | 1Password |
| --- | --- | --- | --- | --- | --- |
| Séquestre de clé | le serveur inscrit l'utilisateur à la récupération d'une organisation, avec une clé publique fabriquée | 3 | 1 | – | – |
| Chiffrement du coffre | champs chiffrés un par un, sans lien avec leur élément : échange de champs, métadonnées en clair, paramètres KDF non authentifiés | 4 | 5 | 1 | 1 |
| Partage | clés publiques non authentifiées | 2 | 1 | 1 | 1 |
| Rétrocompatibilité | repli forcé vers un ancien format sans intégrité | 3 | – | 4 | – |

**Les anti-patterns relevés, et la réponse de la v1 :**

| Anti-pattern | Réponse |
| --- | --- |
| Chiffrement sans intégrité, repli vers un ancien format | AES-GCM partout, comme aujourd'hui ; aucun ancien format, jamais de repli |
| Champs chiffrés un par un, sans lien avec leur élément | deux parties seulement par élément, chacune liée par son `aad` au coffre, à l'élément et à la version de clé (§ 5.4) |
| Réglages de sécurité non authentifiés (KDF, version du compte) | bornes KDF côté client ; pas de version de compte : la Secret Key fait partie du protocole, un serveur ne peut pas annoncer un compte sans elle (§ 5.2) |
| Séquestre, récupération par un tiers | clé de secours seulement (§ 5.7) |
| Suppression ou injection d'éléments (1Password) | manifeste (§ 5.6) |
| Clés publiques non authentifiées | sans objet en v1 ; règle pour le partage au § 9 |

### 3.4 Compatibilité avec l'API Bitwarden

Écartée le 2026-11-07 (§ 10).

---

## 4. Modèle de menace (v1)

| Adversaire | Ce qu'il peut | Ce qu'il ne doit pas pouvoir |
| --- | --- | --- |
| Serveur malveillant, ou pris | voir les métadonnées du § 6.4 ; refuser le service ; servir un état ancien mais cohérent à un Mac neuf (§ 5.6) | lire, modifier ou injecter un élément ; deviner le mot de passe ; cacher, supprimer ou remettre à une ancienne version un élément sans être vu par un Mac qui a un état |
| Fuite de la base, ou d'une sauvegarde | lire les métadonnées | quoi que ce soit d'attaquable par force brute |
| Code qui a `dataKey` : Matcha détourné, coffre verrouillé | voir qu'il existe des éléments, leur nombre et leurs dates | lire un titre, une adresse ou un secret ; supprimer ou modifier un élément sans être vu (§ 5.6) |
| Mac volé, éteint ou session fermée (FileVault) | rien | ouvrir le coffre |
| Mac volé, session ouverte, coffre verrouillé | essayer des mots de passe contre le cache local (PBKDF2) | ouvrir le coffre sans le mot de passe |
| Page web malveillante | afficher un faux formulaire, superposer des calques | obtenir un remplissage pour une autre origine, ou par un clic piégé (§ 8) |
| Qui garde la base ou le trafic, avec un ordinateur quantique plus tard | casser l'échange de clés TLS des Mac en macOS 15 | lire un élément ; deviner le mot de passe (§ 5.8) |

**Limites admises :**
- un Mac compromis pendant que le coffre est ouvert perd ce qui est déverrouillé, comme partout ;
- une mise à jour malveillante de Matcha perd tout ;
- un mot de passe tapé à la main sur un faux site : seul le remplissage par origine protège.

---

## 5. Cryptographie de la v1

### 5.1 Primitives : uniquement ce que Matcha a déjà

| Usage | v1 | Dans Matcha |
| --- | --- | --- |
| Mot de passe → clé | PBKDF2-HMAC-SHA256, de 600 000 à 10 000 000 itérations (§ 3.1 de SYNC.md) | `SyncCrypto.pbkdf2` (CommonCrypto) |
| Sous-clés | HKDF-SHA256 | `SyncCrypto.subkey` |
| Chiffrement | AES-256-GCM, nonce aléatoire de 12 octets | `SyncCrypto.seal`, `open` |
| Complément | ISO/IEC 7816-4 | `SyncCrypto.pad` |
| Manifeste, identifiants calculés | HMAC-SHA256 | CryptoKit |
| TOTP | HMAC-SHA1, SHA-256 ou SHA-512 (RFC 6238) | CryptoKit (`Insecure.SHA1` pour HMAC-SHA1) |
| Touch ID | clé P-256 de la Secure Enclave, avec un contrôle d'accès biométrique | CryptoKit (`SecureEnclave.P256`), LocalAuthentication : nouveau, mais dans les frameworks |
| Génération des clés SSH | Ed25519 (`Curve25519.Signing`), ECDSA P-256 et P-384 (CryptoKit), RSA (Security) | nouveau, dans les frameworks |
| Aléa | générateur du système | `SyncCrypto.randomBytes` |

**Tout ce tableau existe sous macOS 15**, la plupart depuis macOS 10.15 ou 11 : CryptoKit (AES-GCM, HKDF, HMAC, `SecureEnclave.P256`, `Curve25519`), CommonCrypto (PBKDF2), Security (`SecAccessControlCreateWithFlags`, RSA), LocalAuthentication. Il en va de même pour le remplissage (`WKContentWorld`, macOS 11) et le presse-papiers (§ 8). Aucune `#available` n'est nécessaire.

**Écarté en v1, et pourquoi :**
- **Argon2id** n'est pas dans les frameworks d'Apple. La Secret Key rend PBKDF2 suffisant (§ 5.2).
- **OPAQUE** (RFC 9807) n'a pas de bibliothèque Swift. On garde `authKey`, haché par Argon2id côté serveur. Avec la Secret Key, ce hachage ne s'attaque plus.
- **Chiffrement à clé publique :** il ne sert qu'au partage (§ 9). Sans lui, la v1 est entièrement symétrique, donc déjà post-quantique (§ 5.8).
- **Engagement de clé** (contre les chiffrés qui s'ouvrent sous deux clés) : il ne compte que quand plusieurs personnes ont des clés, donc avec le partage. L'octet de version du format le permettra.

### 5.2 La Secret Key

Elle est spécifiée au § 3.2 de SYNC.md (étape 0, faite) :

- **Ce qu'elle est :**
  - 128 bits tirés sur le premier Mac, et de nouveau à chaque récupération ;
  - affichée comme la clé de secours, précédée de `S1-`, avec une somme de contrôle de 4 octets : une faute de frappe est refusée sur le Mac (`S1-R30W-5GY4-…`) ;
  - gardée dans le trousseau de chaque Mac (`SyncKeychain`, jamais synchronisée par iCloud), jamais envoyée ;
  - imprimée dans le kit de secours, avec l'adresse du serveur, l'identifiant et la clé de secours.
- **Dérivation**, avec ses vecteurs au § 3.10 de SYNC.md :
  - `masterKey` = PBKDF2(mot de passe NFKD, `salt`, `iterations`), comme avant ;
  - `authKey` = HKDF(`masterKey` ‖ `secretKey`, "matcha.auth.v1") et `wrapKey` = HKDF(`masterKey` ‖ `secretKey`, "matcha.wrap.v1").
- **Effet :** sans la Secret Key, `authKey` ne permet aucun essai (2¹²⁸). La limite connue du § 3.9 de SYNC.md disparaît, et abaisser les itérations jusqu'à 600 000 ne sert plus à rien. Il ne reste au KDF qu'un rôle : protéger le cache d'un Mac volé avec sa session ouverte. C'est ce qui permet de garder PBKDF2.
- **Pas de rétrogradation possible.** Il n'existe qu'une dérivation, et le client ne sait pas se connecter sans Secret Key : un serveur malveillant n'a aucun moyen d'obtenir une `authKey` qui s'attaque.
- **Contreparties :** un nouveau Mac demande la Secret Key (saisie depuis le kit). Perdre la Secret Key **et** tous ses Mac, c'est devoir passer par la clé de secours.

**Pour tout le compte (D8), sans migration (D9).** La synchro n'est pas encore en production : la Secret Key entre directement dans le protocole de base (§ 3 de SYNC.md), avant la mise en production.
- **Côté serveur, rien ne change :** il ne voit que `authKey`, et la dérivation est faite par le client.
- **Côté client :** Matcha (`SyncAccountKeys`, trousseau, formulaire de connexion, kit) et le client de test Go.
- **Les vecteurs du § 3.10** de SYNC.md sont recalculés.
- **Les comptes de développement** sont recréés.

Il n'y a donc ni version de compte, ni route de passage, ni compte sans Secret Key à gérer.

**Écarté : la Secret Key pour le coffre seul.** Rien à migrer, mais un mot de passe deviné par le serveur, ou plus tard grâce au trafic TLS enregistré (§ 5.8), ouvrirait le compte et les données de navigation. Le serveur apprendrait aussi le mot de passe, peut-être réutilisé ailleurs.

### 5.3 Hiérarchie des clés

```
mot de passe ─PBKDF2─► masterKey ─┐
                                  ├─HKDF─┬─► authKey        (envoyée ; le serveur n'en garde qu'un hachage Argon2id)
Secret Key ───────────────────────┘      └─► wrapKey ─┬─► accountKey     (navigation ; dans le trousseau, comme aujourd'hui)
                                                      └─► vaultRootKey   (coffre ; jamais au repos en clair)
clé de secours ─► recoveryWrapKey ─┬─► accountKey
                                   └─► vaultRootKey

vaultRootKey ─► vaultKey   (une par coffre, enveloppée dans un enregistrement vaultKey)
vaultKey     ─► itemKey    (une par élément) ─► overview, details
itemKey      ─► fileKey    (une par fichier)
Touch ID : vaultRootKey enveloppée par une clé de la Secure Enclave de ce Mac
```

- **`vaultRootKey`, distincte de `accountKey` :** la synchro continue en tâche de fond, et le coffre se verrouille. Ses deux enveloppes (mot de passe, clé de secours) vivent sur le serveur à côté de celles de `accountKey`, et changent avec elles (§ 6.3).
- **`vaultKey` aléatoire, enveloppée par `vaultRootKey`,** plutôt que dérivée : on peut ajouter plus tard d'autres coffres, ou un coffre partagé, sans changer le format.
- **Clé par élément :** déplacer un élément d'un coffre à l'autre ne réenveloppe que sa clé. Les fichiers ont leur propre clé, citée par l'élément.
- **Une clé par usage**, dérivée par HKDF avec des libellés distincts : clé du manifeste (§ 5.6)…

### 5.4 Deux couches de chiffrement

| Couche | Clé | Contient | Qui l'ouvre |
| --- | --- | --- | --- |
| Externe : l'enregistrement de SYNC.md, inchangé (§ 3.5) | `dataKey` | `type`, `schema`, `modifiedAt`, `modifiedBy`, `deleted`, et dans `fields` : `vaultId`, `keyVersion`, puis la couche interne | Matcha, même coffre verrouillé ; jamais le serveur |
| Interne | `itemKey`, enveloppée par `vaultKey` | *overview* (titre, adresses, tags, favori) et *details* (identifiants, secrets, notes, fichiers, historique) | Matcha, coffre déverrouillé |

`fields` d'un `vaultItem` :

```json
{
  "vaultId": "8C1F5E2A-…",
  "keyVersion": 1,
  "itemKey": "base 64 : Seal(vaultKey, itemKey, aad item-key)",
  "overview": "base 64 : Seal(itemKey, overview complété, aad overview)",
  "details": "base 64 : Seal(itemKey, details complétés, aad details)"
}
```

- **`aad`** de chaque partie : `matcha.vault.<partie>.v1|<vaultId>|<itemId>|<keyVersion>`, avec `partie` = `item-key`, `overview` ou `details`. Rien ne se recopie d'un élément ou d'une partie à l'autre : c'est la parade à l'échange de champs (§ 3.3).
- **Pourquoi c'est la bonne forme ici :**
  - le moteur de synchro marche tel quel, coffre verrouillé : lecture, envoi, curseur, conflits sur (`modifiedAt`, `modifiedBy`) ;
  - le serveur ne voit rien de nouveau : un `vaultItem` est un enregistrement comme un autre, type compris ;
  - le cache local chiffré vient tout seul : coffre verrouillé, Matcha garde la couche interne telle qu'elle est arrivée ;
  - les anciennes versions de Matcha gardent un `vaultItem` sans le toucher : c'est un type inconnu (§ 2.2 de SYNC.md).
- **Pourquoi deux parties :** la liste et la recherche n'ouvrent que les *overviews*. Les secrets ne sont déchiffrés qu'à l'usage, et restent moins longtemps en mémoire.
- **Complément :** la taille trahit la catégorie (une carte n'a pas la taille d'une clé RSA). La couche externe est déjà complétée à 256 octets. En plus, l'*overview* est complété à 256 octets, et les *details* à 1 Kio au moins, puis selon **Padmé**, qui coûte au plus 12 % et ne laisse fuir que O(log log n) bits de la taille.
- **La couche externe ne suffit pas à authentifier un élément.** Avec `dataKey` seule, on ne peut rien lire ni fabriquer, mais on peut écrire une suppression (`deleted: true`, sans `fields`), ou remettre une couche interne plus ancienne sous une date nouvelle. C'est le rôle du manifeste (§ 5.6).

Les deux autres types :
- **`vaultKey`** (identifiant calculé `vault-key:<vaultId>`, § 2.2 de SYNC.md) : la `vaultKey` enveloppée par `vaultRootKey`, avec sa version ;
- **`vaultManifest`** (identifiant calculé `vault-manifest:<vaultId>`) : § 5.6.

### 5.5 Verrouillage et Touch ID

- **Déverrouiller par le mot de passe :** PBKDF2, avec la Secret Key du trousseau, donne `wrapKey`, qui ouvre `vaultRootKey`. Une copie de son enveloppe reste sur le Mac, pour déverrouiller hors ligne.
- **Touch ID :** `vaultRootKey` est enveloppée par une clé P-256 de la Secure Enclave, avec un contrôle d'accès `biometryCurrentSet`. Elle devient inutilisable si les empreintes changent, et le mot de passe est alors redemandé. C'est la Secure Enclave qui fait respecter Touch ID, pas une simple vérification par `LAContext`, qu'un code dans le processus pourrait sauter.
- **À vérifier :** SYNC.md § 3.8 note que le trousseau protégé par l'appareil demande un profil de provisionnement, que Matcha n'a peut-être pas. Une clé de la Secure Enclave créée par CryptoKit donne une `dataRepresentation` que l'app garde elle-même : elle ne devrait pas en avoir besoin. À confirmer dans le bac à sable, avec la signature de Matcha.
- **Verrouillage automatique :** après une inactivité réglable, à la veille, au verrouillage de l'écran, au changement d'utilisateur, et par « Verrouiller maintenant ». Les clés et les éléments déchiffrés sont alors oubliés.
- **Mot de passe redemandé** pour afficher un secret marqué comme tel (phrase de récupération, carte), pour exporter, ou pour supprimer le coffre.

### 5.6 Le manifeste du coffre

**But :** qu'aucun élément ne puisse être caché, supprimé ou remis à une version ancienne sans être vu. Ni par le serveur, ni par du code qui n'a que `dataKey`.

- **Contenu :** un enregistrement `vaultManifest` par coffre. Il porte un compteur, et pour chaque élément son identifiant et l'empreinte de sa couche interne. Le tout est authentifié par un HMAC avec une clé tirée de `vaultKey`.
- **Écriture :** les éléments et le manifeste partent dans le même **lot atomique**, tout ou rien. Aujourd'hui, `push` applique chaque changement à part (§ 4.6 de SYNC.md) ; c'est l'un des rares changements du serveur (§ 6.3). Deux Mac qui écrivent en même temps entrent en conflit sur le manifeste, fusionnent et recommencent.
- **Lecture :** un élément n'est accepté que si le manifeste valide le cite avec cette empreinte. Une suppression n'est appliquée que si le manifeste ne cite plus l'élément. Coffre verrouillé, la vérification attend le déverrouillage.
- **Taille :** pour 10 000 éléments, environ 500 Kio. Il faut 256 seaux par préfixe d'identifiant, sous une racine authentifiée, pour ne réécrire qu'un seau, ou un plafond d'éléments en v1. À trancher dans la spec.
- **Ce qu'il détecte :**
  - un élément caché à un Mac et montré à un autre ;
  - une suppression ou une version ancienne glissée par quelqu'un qui n'a que `dataKey` ;
  - le retour en arrière d'un seul élément.
- **Ce qu'il ne détecte pas :** un serveur qui fige tout le coffre pour un Mac, ou sert un état ancien mais complet à un Mac neuf. Chaque Mac garde le plus grand compteur vu, et l'interface affiche « Dernière modification du coffre le … depuis … » pour qu'on le remarque.

Ce mécanisme pourrait servir plus tard aux données de navigation (la question « journal signé des versions » du § 8 de SYNC.md).

### 5.7 Récupération

- **La clé de secours** (existante) enveloppe aussi `vaultRootKey`, et suffit seule à récupérer le compte et le coffre (§ 3.7 de SYNC.md). Avec la Secret Key, la récupération se termine par un nouveau mot de passe ; la Secret Key est gardée ou tirée de nouveau, avec un nouveau kit.
- **Pas de séquestre, pas de récupération par un tiers.** C'est la première catégorie d'attaques de l'ETH. L'accès d'urgence viendra peut-être avec le partage (§ 9).

### 5.8 Post-quantique

**Le risque :** « récolter maintenant, déchiffrer plus tard ». Le serveur, une fuite de la base, ou quiconque enregistre le trafic, peut tout garder aujourd'hui et attendre un ordinateur quantique. Celui-ci casse X25519, P-256 et RSA (algorithme de Shor). Il ne casse pas la crypto symétrique : l'algorithme de Grover divise au mieux par deux la sécurité en bits, et une clé de 256 bits en garde 128.

**La règle (D7) :** aucun secret qui dure n'est protégé par de la crypto à clé publique classique seule.

**La v1 la respecte déjà, sous macOS 15.** Tout ce que garde le serveur est chiffré en symétrique, par des clés tirées du mot de passe et de la Secret Key : AES-256-GCM, HKDF, HMAC, PBKDF2. Il n'y a rien à rendre post-quantique, et rien à réserver à macOS 26. La Secret Key fait 128 bits : Grover demanderait 2^64 calculs de PBKDF2, les uns après les autres, dans un ordinateur quantique.

**Restent deux briques à clé publique, en dehors des données du serveur :**

| Brique | Ce qui se récolte | Réponse |
| --- | --- | --- |
| TLS | l'échange de clés des Mac en macOS 15 (X25519 ou P-256). Dedans : `authKey`, les jetons, et des enregistrements déjà chiffrés | **la Secret Key** (D8). Avec elle, `authKey` ne permet pas de deviner le mot de passe : la récolte ne donne rien à déchiffrer. `authKey` permettrait quand même plus tard de se connecter, si le mot de passe n'a pas changé : pour effacer, pas pour lire, et le manifeste le voit pour le coffre (§ 5.6). Sous macOS 26, `URLSession` propose X25519MLKEM768 par défaut. Le serveur ne fait pas TLS lui-même : il faut nginx avec OpenSSL 3.5 ou plus |
| Touch ID | rien côté serveur. L'enveloppe de `vaultRootKey` par la clé P-256 de la Secure Enclave reste sur le Mac (§ 5.5) | il faudrait une copie du disque *et* un ordinateur quantique. Sous macOS 26, la Secure Enclave fait aussi ML-KEM : chaque Mac choisit selon sa version (`#available`), sans effet sur les autres |

**Réserver le coffre à macOS 26 ?** Non. Ça n'apporterait que le TLS post-quantique, que la Secret Key rend inutile ici, et ça priverait de coffre les Mac en macOS 15.

**Le partage, lui, en aura besoin** (§ 9) : la clé d'un espace part vers un autre compte, chiffrée pour sa clé publique. On utilisera X-Wing (`draft-connolly-cfrg-xwing-kem`). **C'est un algorithme, pas une bibliothèque d'Apple** : ML-KEM-768 et X25519, combinés par SHA3-256. CryptoKit l'a sous macOS 26 (`XWingMLKEM768X25519`, aussi dans HPKE), et Go dans `crypto/hpke`. Les octets sont les mêmes partout. Sous macOS 15, deux voies :

1. **L'assembler avec mlkem-native** (recommandé) :
   - c'est ML-KEM et SHA-3/SHAKE en C90, par le PQ Code Package (Linux Foundation) ;
   - la mémoire et les types sont prouvés par CBMC, l'assembleur AArch64 et x86_64 avec HOL Light ; il est testé contre les vecteurs ACVP et Wycheproof ;
   - c'est l'implémentation de ML-KEM de liboqs, d'AWS-LC et de rustls ; licence Apache-2.0, ISC ou MIT ;
   - X25519 vient de CryptoKit (`Curve25519.KeyAgreement`, macOS 10.15).

   Un Mac en macOS 15 produit et ouvre les mêmes enveloppes qu'un Mac en macOS 26 qui passe par CryptoKit. Il n'y a ni double enveloppe, ni Mac laissé de côté, et la bibliothèque part quand Matcha abandonne macOS 15. Le prix : une dépendance en C, seule exception à D4, limitée au partage.
2. **Réserver le partage à macOS 26.** Rien à ajouter, mais il faut un Mac en macOS 26 pour inviter, accepter une invitation et retirer un membre (la rotation chiffre la nouvelle clé pour chaque membre). Une fois l'invitation acceptée, la clé de l'espace est enveloppée en symétrique par `vaultRootKey` : les Mac en macOS 15 du même compte lisent l'espace. Après une rotation, ils attendent qu'un Mac en macOS 26 du compte l'ait reçue. Une personne qui n'a que des Mac en macOS 15 ne peut pas être invitée.

**Écartés :**
- **swift-crypto :** sur Mac, il réexporte CryptoKit, donc avec les mêmes versions de macOS. Son implémentation en BoringSSL n'est compilée qu'en dehors des plateformes d'Apple (dans son `Package.swift`).
- **liboqs :** bien plus vaste, et il utilise déjà mlkem-native pour ML-KEM.
- **BoringSSL embarqué :** il a ML-KEM et X-Wing, mais c'est une grosse bibliothèque, dont il faudrait renommer les symboles.
- **ML-KEM seul, sans X25519 :** l'hybride protège encore si ML-KEM, plus jeune, a une faille.

**Les signatures n'ont pas la même urgence** (paquets de clés et droits signés, § 9). Une signature ne s'attaque qu'au moment où on la vérifie, et rien ne se récolte : Ed25519 suffit au départ. Il faudra passer à ML-DSA, en hybride, avant qu'un ordinateur quantique existe. C'est la même situation : CryptoKit l'a sous macOS 26, et le même projet a mldsa-native.

---

## 6. Synchro et serveur en v1

### 6.1 Règles de synchro propres au coffre

- **Aucun secret n'est perdu.** En cas de conflit, la fusion se fait champ par champ, sur le modèle des notes (§ 2.10 de SYNC.md, `SyncFieldMerge`, `SyncNoteConflict`) :
  - la valeur perdante d'un mot de passe ou d'un secret passe dans l'historique de l'élément ;
  - le texte perdant d'une note devient une copie ;
  - une modification l'emporte sur une suppression simultanée.
- **Conflit coffre verrouillé :** la fusion demande la couche interne. Un changement local en conflit attend donc le déverrouillage, avec la version du serveur. En général, il part avant que le coffre ne se verrouille.
- **Corbeille :** supprimer, c'est mettre `trashedAt` dans les *details*. La vraie suppression (`deleted: true`, et le manifeste mis à jour) part au bout de 30 jours, envoyée par n'importe quel Mac déverrouillé.
- **Opérations sur tout le compte** (« Remplacer les données synchronisées », « Remplacer les données de ce Mac », première synchro, § 5 de SYNC.md) : elles **ne touchent jamais au coffre**. Un Mac dont le coffre est verrouillé, ou pas encore ouvert, effacerait sinon le coffre de tous les autres. Le coffre se traite comme les types inconnus d'aujourd'hui.
- **Garde-fou contre les suppressions en masse :** plus strict pour le coffre, avec une confirmation dès quelques éléments.
- **Curseur expiré :** les règles habituelles, sans changement. Les éléments du coffre repartent comme les autres.
- **Horloge :** les règles du § 2.8 de SYNC.md. Un Mac à l'heure fausse donne de faux codes TOTP : le dire.

### 6.2 Fichiers : une suite de paquets

- Un fichier est découpé en **morceaux de 16 Mio au plus**, sous la limite des paquets (`maxBlobBytes`, 21 Mio). Chaque morceau est un paquet (`PUT /v1/blobs/{id}`), avec un identifiant aléatoire.
- Chaque morceau est chiffré par la `fileKey` du fichier, avec l'`aad` `matcha.vault.file.v1|<fileId>|<index>|<nombre de morceaux>`. Un morceau manquant, déplacé ou d'un autre fichier ne s'ouvre pas, comme dans la construction STREAM. Les *details* de l'élément citent `{ fileId, name, type, size, sha256, key, chunks: [blobId…] }`.
- **Rien ne change sur le serveur.**
  - Les morceaux partent avant l'élément qui les cite, avec le suivi des paquets en attente (§ 4.7 de SYNC.md).
  - Ils comptent dans `maxAccountBytes`.
  - Ils se téléchargent à la demande, avec une option « Garder hors ligne ».
  - Ils sont complétés à 64 Kio, comme les paquets.
- **Taille maximale proposée :** 100 Mio par fichier en v1 (R6).
- **Aperçus :** calculés par Matcha, gardés chiffrés dans son cache.

### 6.3 Ce qui change sur le serveur

| Changement | Pourquoi | Taille |
| --- | --- | --- |
| `accounts` : `wrapped_vault_root_key`, `recovery_wrapped_vault_root_key` ; renvoyées dans la Session ; prises par `account/password`, `account/recovery` et `recover/finish`, en même temps que les enveloppes de `accountKey` | même cycle de vie que `wrappedAccountKey` | petit |
| `PUT /v1/account/vault` (avec `authKey`) : créer les enveloppes, à l'activation du coffre | activation | petit |
| `push` : option `atomic` (tout ou rien, dans la transaction déjà ouverte sous `WithAccountLock`) | manifeste (§ 5.6) | petit à moyen |
| `GET /v1/server` : `features` contient `vault` | le client cache le coffre sans lui | trivial |
| Audit : `vault.enable` | journal de sécurité | trivial |
| Éléments, fichiers | rien : contenus opaques, paquets existants | — |
| Secret Key | rien : seul le client la connaît (§ 5.2) | — |

### 6.4 Ce que voit le serveur

Il voit la même chose qu'aujourd'hui (§ 3.9 de SYNC.md), plus deux enveloppes. Il ne sait pas quels enregistrements appartiennent au coffre.

**Fuites admises :**
- le nombre total d'enregistrements et leurs tailles arrondies ;
- les horaires : un élément enregistré au moment où l'utilisateur s'inscrit sur un site ;
- des paquets envoyés ensemble, ce qui laisse deviner un fichier et sa taille approximative.

---

## 7. Modèle des éléments

**S'aligner sur le Credential Exchange Format de la FIDO Alliance (CXF)** : *proposed standard* d'août 2025, errata de mars 2026. C'est le format d'échange entre gestionnaires (le protocole CXP l'utilise : Apple Passwords, Bitwarden, 1Password, Dashlane…).
- Il définit les types `address`, `api-key`, `basic-auth`, `credit-card`, `custom-fields`, `drivers-license`, `file`, `generated-password`, `identity-document`, `item-reference`, `note`, `passkey`, `passport`, `person-name`, `ssh-key`, `totp` et `wifi`.
- Un élément y porte une **liste de credentials** : un identifiant, c'est `basic-auth` + `totp`, et plus tard `passkey`.
- Il a des extensions nommées (`<rpId>/<nom>`), et des règles de compatibilité proches des nôtres : champs et valeurs inconnus ignorés, type inconnu gardé.
- Ce qui manque devient une extension `matchabrowser.com/…` : compte bancaire, portefeuille crypto, serveur, licence, codes de secours.
- L'import et l'export suivent presque d'eux-mêmes.

*Overview* et *details* d'un identifiant, une fois ouverts :

```json
{
  "overview": {
    "category": "login",
    "title": "GitHub",
    "subtitle": "jeremy",
    "favorite": false,
    "tags": ["dev"],
    "scope": { "urls": ["https://github.com/login"], "match": "site" }
  },
  "details": {
    "credentials": [
      { "type": "basic-auth", "username": "jeremy", "password": "…" },
      { "type": "totp", "secret": "…", "algorithm": "sha1", "digits": 6, "period": 30, "issuer": "GitHub" }
    ],
    "fields": [{ "id": "…", "label": "PIN", "type": "concealed", "value": "…" }],
    "notes": "…",
    "files": [{ "fileId": "…", "name": "recovery-codes.pdf", "type": "application/pdf", "size": 48213, "sha256": "…", "key": "…", "chunks": ["…"] }],
    "history": [{ "field": "password", "value": "…", "at": "2026-10-17T08:30:12.345Z", "by": "3F2A8C1E-…" }],
    "createdAt": "2026-10-17T08:30:12.345Z",
    "trashedAt": null
  }
}
```

- **Historique :** dans les *details*, borné, par exemple aux 10 dernières valeurs des mots de passe et des secrets, avec leur date et leur Mac. Le serveur continue de ne garder que la dernière version (R5).
- **Clé SSH :** clé privée au format OpenSSH, sans phrase de passe (le coffre la protège), clé publique, empreinte SHA-256, commentaire. Algorithmes : Ed25519 par défaut, ECDSA P-256 et P-384, RSA d'au moins 3 072 bits. Une clé importée avec une phrase de passe est déchiffrée à l'import.
- **`schema` et types inconnus :** les règles de SYNC.md § 2.2 et § 2.9 valent pour la couche interne. Une catégorie inconnue, ou un `schema` plus récent, est montrée en lecture seule et n'est jamais réécrite.

---

## 8. Dans Matcha

**Remplissage :**
- **L'interface du remplissage est native** (barre d'adresse, barre latérale, feuille), **jamais dans la page**. Aucun calque d'une page ne peut la cacher ni la superposer. Matcha évite ainsi le *clickjacking* d'extension (Marek Tóth, DEF CON 33, août 2025 : 10 extensions de gestionnaires sur 11 touchées).
- Les scripts qui lisent et remplissent les formulaires tournent dans un `WKContentWorld` à part, que la page ne voit pas.
- Rien n'est rempli au chargement : seulement après une action de l'utilisateur (clic dans l'interface native, raccourci).
- **Correspondance :** par défaut, le même site, d'après la **liste publique des suffixes complète, section privée comprise** (`github.io`, `herokuapp.com`… sont des suffixes). Elle est embarquée et mise à jour avec l'app. Ce n'est pas `SiteDomain`, l'approximation du § 2.7 de SYNC.md, qui reste pour les zooms et les notes (format figé). On peut aussi choisir, par élément, « cet hôte » ou « cette adresse exacte ».
- **Cadres :** un cadre d'une autre origine n'est rempli que si **sa** propre origine correspond à l'élément.
- `http` : un avertissement, et aucun remplissage d'un identifiant enregistré en `https`.
- **Enregistrer :** après l'envoi d'un formulaire de connexion, une barre native propose d'enregistrer ou de mettre à jour l'identifiant. L'ancien mot de passe passe dans l'historique.
- **Fenêtres privées :** le remplissage y marche, mais rien n'y est proposé à l'enregistrement sans le demander.

**Sécurité locale :**
- **Presse-papiers :**
  - types `org.nspasteboard.ConcealedType` et `TransientType`, ignorés par les gestionnaires de presse-papiers ;
  - `currentHostOnly`, pour que rien ne passe par le presse-papiers universel ;
  - effacement au bout de 90 secondes si le contenu n'a pas changé.
- **Écran :** les fenêtres qui montrent un secret ont `sharingType = .none`. À vérifier sous macOS 15 : les captures par ScreenCaptureKit ne respecteraient plus toujours ce réglage. Dans ce cas, le secret reste masqué par défaut et ne s'affiche qu'à la demande.
- **Traces :** aucun secret dans les journaux, les rapports de plantage, Spotlight, Quick Look ou les caches d'images.
- **Générateur :** mots de passe et phrases de passe (liste de mots de l'EFF), générateur du système, jauge de `SyncPasswordStrength`.
- **Analyse :** mots de passe faibles et réutilisés, calculés sur le Mac. La vérification des fuites (HIBP en k-anonymat) est en option, plus tard.
- **Import :** CSV (Chrome, Safari, Firefox), JSON de Bitwarden, 1PUX de 1Password, CXF. **Export :** CXF chiffré ; en clair, seulement avec un avertissement et le mot de passe.

---

## 9. Plus tard

**Client annexe** (app macOS à part, ou assistant livré avec Matcha) :
- **Agent SSH :**
  - socket dans un conteneur de groupe (compatible avec le bac à sable, comme 1Password), et `IdentityAgent` dans `~/.ssh/config` ;
  - approbation à l'usage par Touch ID ; la clé ne sort jamais de l'agent ;
  - signature des commits Git.

  Il doit marcher sans Matcha ouvert.
- **Fournisseur AutoFill de macOS** (`ASCredentialProviderViewController`, passkeys depuis macOS 14) : mots de passe et passkeys dans Safari, Chrome et les apps, sans extension. Il faut vérifier aussi comment `WKWebView` traite WebAuthn dans un navigateur tiers, pour les passkeys des pages de Matcha.
- **CLI** : `run` et références à des champs, à la `op run`. Le Go de `internal/testclient` en est une base possible.

**Extension de navigateur**, si elle vient :
- c'est une seconde implémentation de la crypto et des règles de fusion (TypeScript), vérifiée par les vecteurs ;
- le *clickjacking* impose une interface hors de la page ;
- MV3 arrête son *service worker*, et les clés en mémoire avec lui ;
- c'est le moment de reposer la question d'un code commun.

**Partage :**
- **Des espaces côté serveur :** `records` passe de (`account_id`, `id`) à (`space_id`, `id`), avec des membres et des clés enveloppées. L'espace actuel du compte reste celui de la navigation et du coffre personnel.
- **Des clés publiques authentifiées :** un paquet de clés signé, l'empreinte vérifiée hors bande (invitation par code), l'épinglage, et une alerte si une clé change. C'est la catégorie d'attaques « partage » de l'ETH, qui touche les quatre gestionnaires.
- **Des enveloppes post-quantiques (X-Wing) dès le premier jour :** avec mlkem-native sous macOS 15, ou un partage réservé à macOS 26 (§ 5.8, R9). Jamais X25519 seul.
- **La crypto qui va avec :** engagement de clé, droits signés (un lecteur ne peut pas écrire), et rotation de la clé du coffre au retrait d'un membre.
- **L'accès d'urgence**, en option, en disant que le serveur fait respecter le délai.

**Autres :** OPAQUE, Argon2id (si une dépendance devient acceptable), appairage d'un nouveau Mac sans saisir la Secret Key (échange de clés avec une chaîne courte comparée à l'écran, en X-Wing comme le partage, § 5.8), plusieurs coffres personnels.

---

## 10. Décisions

### Décidé le 2026-11-07

| # | Décision | Conséquence |
| --- | --- | --- |
| D1 | Même compte que la synchro | les éléments vivent dans l'espace du compte (§ 5.4) |
| D2 | Pas de noyau commun en Rust : le code existant (Swift dans Matcha, Go côté serveur) | la spec et ses vecteurs restent l'autorité ; le client de test Go est l'implémentation indépendante |
| D3 | Partage plus tard | pas d'espaces, pas de clés publiques en v1 ; le format le permettra (`vaultId`, `vaultKey` enveloppée, octet de version) |
| D4 | Compatible macOS 15, comme Matcha ; rien d'autre que les frameworks d'Apple | PBKDF2 au lieu d'Argon2id, `authKey` au lieu d'OPAQUE, pas de crypto à clé publique en v1 |
| D5 | Intégré à Matcha ; client annexe plus tard | v1 sans agent SSH ni fournisseur AutoFill |
| D6 | Pas de compatibilité avec l'API Bitwarden | — |
| D7 | Post-quantique obligatoire : aucun secret qui dure sous de la crypto à clé publique classique seule | la v1 l'est déjà, sans macOS 26, car elle est symétrique ; le partage et l'appairage passent par X-Wing (§ 5.8) |
| D8 | Secret Key pour tout le compte (ex-R1) | la connexion à la synchro en dépend aussi ; PBKDF2 suffit, et le trafic TLS enregistré ne donne rien (§ 5.2, § 5.8) |
| D9 | Pas de migration : la synchro n'est pas en production (ex-R10) | la Secret Key entre dans le § 3 de SYNC.md avant la mise en production ; rien côté serveur ; vecteurs recalculés, comptes de développement recréés ; ni version de compte, ni route de passage (§ 5.2) |

### Restent à trancher

| # | Question | Recommandé |
| --- | --- | --- |
| R2 | Manifeste et `push` atomique en v1 ? | **oui** : l'ajouter après coup est plus dur, et c'est la parade aux suppressions faites avec `dataKey` seule |
| R3 | Un coffre ou plusieurs en v1 ? | **un seul**, avec tags et favoris ; le format en permet plusieurs |
| R4 | Catégories de la v1 | identifiant (avec TOTP), note, carte, identité, document, clé SSH (stockage) ; les autres par le modèle générique |
| R5 | Historique | **dans les *details*, borné**, avec une corbeille de 30 jours |
| R6 | Taille maximale d'un fichier | **100 Mio**, téléchargement à la demande |
| R7 | Correspondance des sites | **liste publique des suffixes complète**, embarquée |
| R8 | Verrouillage par défaut | par exemple 15 minutes d'inactivité, veille et écran verrouillé ; Touch ID proposé à l'activation |
| R9 | Partage sous macOS 15 : X-Wing assemblé avec mlkem-native, ou partage réservé à macOS 26 ? À trancher au moment du partage | **mlkem-native** : les mêmes enveloppes partout, et personne de laissé de côté (§ 5.8) |

### À vérifier

- La clé de la Secure Enclave sans profil de provisionnement, dans le bac à sable (§ 5.5).
- Le coût de PBKDF2 à 600 000 itérations ou plus à chaque déverrouillage par mot de passe (Touch ID le reste du temps).
- La forme du manifeste (seaux, plafond) face à un coffre de plusieurs milliers d'éléments.
- `sharingType = .none` face à ScreenCaptureKit sous macOS 15 (§ 8).
- X25519MLKEM768 vraiment négocié entre Matcha sous macOS 26 et nginx (OpenSSL 3.5 ou plus) (§ 5.8).
- ML-KEM dans la Secure Enclave sous macOS 26 pour Touch ID : API, et même question de profil de provisionnement (§ 5.8).
- Au moment du partage : le X-Wing assemblé avec mlkem-native donne les mêmes octets que CryptoKit sous macOS 26 et que Go `crypto/hpke`, d'après les vecteurs du draft.

---

## 11. Découpage proposé

Chaque étape est utilisable seule, et chacune a ses tests, comme dans [PLAN.md](PLAN.md).

0. **Secret Key dans la synchro, avant la mise en production** (D8, D9), **faite** (branches `feat/sync-secret-key`). Elle ne dépend pas du coffre, et c'est la seule étape pressée : après la mise en production, il faudrait une vraie migration.
   - SYNC.md § 3 : dérivation, création du compte, connexion d'un autre Mac, changement de mot de passe, récupération, kit, trousseau, ce que voit le serveur (§ 3.9) ; vecteurs du § 3.10 recalculés par une implémentation Python indépendante ;
   - Matcha : `SyncSecretKey`, `SyncAccountKeys`, trousseau, formulaire de connexion, kit, traductions ;
   - client de test Go (`internal/testclient`) et tests de bout en bout ;
   - rien côté serveur.
1. **Spec du coffre**, **faite** le 2026-11-08 ([VAULT.md](VAULT.md), R2 à R8 retenues comme recommandé, revue adversariale intégrée) : trancher R2 à R8. Écrire `VAULT.md` (ou une partie de SYNC.md) au même format :
   - clés, couches, manifeste, règles de synchro, fichiers ;
   - vecteurs calculés par une implémentation indépendante (Python, comme au § 3.10) ;
   - faire relire la crypto.
2. **Serveur** (§ 6.3), **fait** (branche `feat/vault`) :
   - enveloppes de `vaultRootKey` ;
   - `push` atomique ;
   - `features` ;
   - client de test Go avec la crypto du coffre, et tests de bout en bout : deux Mac se partagent un coffre, un serveur qui cache un élément est détecté, une suppression écrite avec `dataKey` seule est refusée.
3. **Matcha, coffre**, **fait** (branche `feat/vault`) :
   - `vaultRootKey`, verrouillage, Touch ID ;
   - types `vaultKey`, `vaultItem`, `vaultManifest` et règles de synchro ;
   - interface du coffre (liste, recherche, édition), générateur, TOTP, presse-papiers.
4. **Matcha, remplissage**, **fait** :
   - liste des suffixes, scripts dans un `WKContentWorld`, cadres ;
   - barre d'enregistrement ;
   - import et export.
5. **Fichiers et clés SSH**, **faits** : suites de paquets ; génération, import et copie de la clé publique.
6. **Plus tard** (§ 9) : client annexe (agent SSH, fournisseur AutoFill, passkeys), partage, extension.

---

## Sources

- Vaultwarden, sources lues pour cette discovery : `src/db/models/{cipher,user,attachment}.rs`, `src/api/core/{ciphers,accounts}.rs`, `src/api/notifications.rs` — <https://github.com/dani-garcia/vaultwarden>
- M. Scarlata, G. Torrisi, M. Backendal, K. G. Paterson, *Zero Knowledge (About) Encryption: A Comparative Security Analysis of Cloud-based Password Managers*, USENIX Security 2026 — <https://eprint.iacr.org/2026/058>, <https://zkae.io/>, <https://www.usenix.org/conference/usenixsecurity26/presentation/scarlata>
- Compte rendu de l'étude : <https://www.infosecurity-magazine.com/news/vulnerabilities-password-managers>, <https://www.theregister.com/2026/02/16/password_managers/>
- Bitwarden, état cryptographique du compte (v2) : <https://sdk-api-docs.bitwarden.com/bitwarden_core/key_management/account_cryptographic_state/index.html>
- FIDO Alliance, *Credential Exchange Format*, proposed standard (août 2025) et errata (mars 2026) : <https://fidoalliance.org/specs/cx/cxf-v1.0-ps-20250814.html>, <https://fidoalliance.org/specs/cx/cxf-v1.0-ps-errata-20260309.html>
- RFC 9807, *The OPAQUE Augmented PAKE Protocol* (juillet 2025) : <https://www.rfc-editor.org/rfc/rfc9807.html>
- Go, `crypto/hpke` : <https://pkg.go.dev/crypto/hpke> ; Apple CryptoKit, HPKE et X-Wing : <https://developer.apple.com/documentation/cryptokit/hpke/ciphersuite>
- X-Wing, *a general-purpose hybrid post-quantum KEM* : <https://datatracker.ietf.org/doc/draft-connolly-cfrg-xwing-kem/>
- mlkem-native (PQ Code Package) : <https://github.com/pq-code-package/mlkem-native> ; mldsa-native : <https://github.com/pq-code-package/mldsa-native> ; vérification chez AWS : <https://www.amazon.science/blog/verifying-and-optimizing-post-quantum-cryptography-at-amazon>
- swift-crypto (`Package.swift` : BoringSSL hors des plateformes d'Apple seulement) : <https://github.com/apple/swift-crypto>
- Apple, TLS 1.3 avec X25519MLKEM768 par défaut dans `URLSession` : <https://support.apple.com/guide/security/sec100a75d12/web> ; annonce de la crypto post-quantique dans CryptoKit : <https://groups.google.com/a/list.nist.gov/g/pqc-forum/c/E9daYYNVihM>
- *DOM-based Extension Clickjacking*, DEF CON 33 : <https://cyberinsider.com/zero-day-clickjacking-flaws-found-in-password-managers-used-by-millions/>
- 1Password Security Design (2SKD, SRP, hiérarchie des clés) : <https://1passwordstatic.com/files/security/1password-white-paper.pdf>
