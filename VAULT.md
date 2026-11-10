# VAULT.md — coffre-fort de Matcha

Spécification du coffre : un gestionnaire de mots de passe, de codes à usage unique, de clés SSH et de fichiers, chiffré de bout en bout et synchronisé par le compte de la synchro. **Version 1, brouillon**, écrite le 2026-11-08 à partir de [VAULT-DISCOVERY.md](VAULT-DISCOVERY.md) : décisions D1 à D9, et R2 à R8 tranchées comme recommandé (§ 13). Elle complète [SYNC.md](SYNC.md), qui reste l'autorité pour le compte, les enregistrements et l'API. Les changements de l'API pour le coffre y sont décrits (§ 3.2, § 4.3, § 4.4, § 4.6 de SYNC.md).

---

## 1. Principes

- **Le serveur ne sait rien,** comme pour la synchro. Le modèle de menace va jusqu'au serveur malveillant (§ 11). Le serveur ne voit pas le type des enregistrements ; seuls les lots atomiques lui laissent deviner lesquels sont ceux du coffre (§ 11).
- **Deux couches.** Un élément du coffre est un enregistrement de la synchro (couche externe, `dataKey`). Il porte une couche interne, chiffrée par des clés que seul le mot de passe, la Secret Key ou la clé de secours ouvrent (§ 2). La synchro tourne coffre verrouillé ; le coffre reste fermé.
- **Rien ne se cache, rien ne se perd.** Un manifeste authentifié par la clé du coffre dit quels éléments existent, dans quelle version, et lesquels ont été supprimés (§ 4). Un Mac qui a vu un élément le garde, et le renvoie si le serveur le perd, le cache ou le remet en arrière (§ 5.3).
- **Que des frameworks d'Apple, compatibles macOS 15** (D4) : toute la crypto est symétrique, donc déjà post-quantique (D7, § 5.8 de la discovery).
- **Un seul coffre personnel en v1** (R3). Le format en permet d'autres, et le partage plus tard (D3).

---

## 2. Clés

### 2.1 Hiérarchie

```
mot de passe, Secret Key ─► wrapKey ─────────┬─► accountKey      (SYNC.md § 3.2)
clé de secours ─► recoveryWrapKey ───────────┘
                     │            │
                     └────────────┴─► vaultRootKey   (jamais au repos en clair)
                                          │
                                          └─HKDF─► vaultKey (coffre personnel)
                                                     ├─HKDF─► itemWrapKey ─► itemKey (une par élément) ─► overview, details
                                                     └─HKDF─► manifestKey ─► manifeste et seaux
                                    itemKey, dans les details ─► fileKey (une par fichier)
Touch ID : vaultRootKey enveloppée par une clé de la Secure Enclave de ce Mac (§ 7.2)
```

| Clé | Obtenue par | Rôle |
| --- | --- | --- |
| `vaultRootKey` | 32 octets aléatoires, tirés à la création du compte | clé racine du coffre ; ne change pas avec le mot de passe |
| `vaultKey` | HKDF(vaultRootKey, "matcha.vault.personal.v1") | clé du coffre personnel |
| `itemWrapKey` | HKDF(vaultKey, "matcha.vault.item-wrap.v1") | enveloppe les clés d'élément |
| `manifestKey` | HKDF(vaultKey, "matcha.vault.manifest.v1") | chiffre et authentifie le manifeste (§ 4) |
| `itemKey` | 32 octets aléatoires, tirés à la création de l'élément, gardés toute sa vie | chiffre l'*overview* et les *details* d'un élément |
| `fileKey` | 32 octets aléatoires, tirés pour chaque fichier ajouté | chiffre les morceaux d'un fichier (§ 6) |

Les notations sont celles de SYNC.md § 3.1 : `HKDF(k, "info")` donne 32 octets (HKDF-SHA256, sel vide), `Seal(k, données, aad)` donne `nonce ‖ chiffré ‖ étiquette` (AES-256-GCM, nonce aléatoire de 12 octets).

### 2.2 `vaultRootKey` et ses enveloppes

`vaultRootKey` est tirée à la création du compte, en même temps qu'`accountKey` (§ 3.3 de SYNC.md), parce que c'est le seul moment où le client a à la fois `wrapKey` et la clé de secours. Tout compte a donc un coffre, utilisé ou non.

Le serveur garde deux enveloppes, à côté de celles d'`accountKey` :

- `wrappedVaultRootKey` = Seal(wrapKey, vaultRootKey, "matcha.vault-root-key.v1")
- `recoveryWrappedVaultRootKey` = Seal(recoveryWrapKey, vaultRootKey, "matcha.vault-root-key.v1")

L'`aad` n'est pas celui d'`accountKey` (`matcha.account-key.v1`) : un serveur qui échangerait les deux enveloppes serait vu, car elles ne s'ouvriraient pas.

Elles suivent les enveloppes d'`accountKey` à chaque étape (SYNC.md § 3.3 à § 3.7) :

| Moment | Ce que fait le client |
| --- | --- |
| Création du compte | tire `vaultRootKey`, envoie les deux enveloppes (`POST /v1/accounts`) |
| Connexion d'un Mac | reçoit `wrappedVaultRootKey` dans la Session ; l'ouvre seulement pour déverrouiller (§ 7.1) |
| Nouveau mot de passe | ouvre `wrappedVaultRootKey` avec l'ancienne `wrapKey`, la réenveloppe avec la nouvelle (`POST /v1/account/password`) |
| Nouvelle clé de secours | ouvre `wrappedVaultRootKey` avec `wrapKey` (le mot de passe est redemandé), l'enveloppe avec la nouvelle `recoveryWrapKey` (`POST /v1/account/recovery`) |
| Récupération | ouvre `recoveryWrappedVaultRootKey` (`recover/start`), la réenveloppe avec la nouvelle `wrapKey` (`recover/finish`) |

Le serveur exige les enveloppes du coffre partout où il exige celles d'`accountKey` : une requête qui en change une sans l'autre est refusée (`invalid_request`). Un mot de passe changé sur une enveloppe et pas sur l'autre rendrait le coffre inaccessible.

`vaultRootKey` ne change jamais en v1. Sa rotation (nouvelle clé, tous les éléments rechiffrés) est réservée par `keyVersion` (§ 3.1).

### 2.3 Le coffre personnel

- **Identifiant** (`vaultId`) : les 16 premiers octets de HKDF(vaultKey, "matcha.vault.id.v1"), mis en forme d'UUID version 8 comme les identifiants calculés (SYNC.md § 2.2). Il est le même sur tous les Mac du compte, le serveur ne peut pas le retrouver, et il ne dépend pas d'`accountKey` : une rotation de la clé de compte (SYNC.md § 3.5) ne change pas les `aad` de la couche interne.
- **Clé** : `vaultKey` est dérivée de `vaultRootKey`, et non tirée. Deux Mac qui ouvrent le coffre en même temps pour la première fois ont donc la même clé : il n'y a ni enregistrement de clé à créer, ni course à départager, ni enregistrement qu'un serveur pourrait remplacer.
- **Plus tard** (partage, plusieurs coffres) : un coffre partagé aura une `vaultKey` aléatoire, enveloppée dans un enregistrement `vaultKey`. Tout ce qui est sous `vaultKey` (éléments, manifeste) reste identique. Le type `vaultKey` est réservé.

---

## 3. Éléments : les deux couches

### 3.1 Couche externe

Un élément est un enregistrement de type `vaultItem` (`schema` 1), d'identifiant aléatoire (UUID v4), chiffré par `dataKey` comme tous les autres (SYNC.md § 3.5). Ses `fields` :

```json
{
  "vaultId": "09250775-D7E9-8622-B187-5873A65BF1C2",
  "keyVersion": 1,
  "itemKey": "base 64",
  "overview": "base 64",
  "details": "base 64"
}
```

- `keyVersion` : la version de `vaultKey`, 1 en v1.
- `itemKey` = `0x01` ‖ Seal(itemWrapKey, itemKey, aad `item-key`).
- `overview` = `0x01` ‖ Seal(itemKey, overview complété, aad `overview`).
- `details` = `0x01` ‖ Seal(itemKey, details complétés, aad `details`).

L'octet `0x01` est la version du format, comme pour les enregistrements. Chaque partie a son `aad` :

```
matcha.vault.<partie>.v1|<vaultId en minuscules>|<id de l'élément en minuscules>|<keyVersion>
```

avec `<partie>` = `item-key`, `overview` ou `details`. Une partie recopiée d'un élément à un autre, d'un coffre à un autre, ou d'une partie à l'autre, ne s'ouvre pas : c'est la parade à l'échange de champs (§ 3.3 de la discovery).

Une suppression définitive est une suppression de la synchro (`deleted: true`, `fields` vide). Elle n'est acceptée que dans les conditions du § 5.

### 3.2 Couche interne

L'*overview* et les *details* sont des objets JSON en UTF-8, aux clés triées (comme SYNC.md § 2.2), sans échapper `/` ni les caractères hors ASCII ; un lecteur accepte tout JSON valide. Il en va de même du manifeste (§ 4.1). L'*overview* sert à la liste et à la recherche, les *details* portent les secrets (§ 9). Ouvrir le coffre n'ouvre que les *overviews* ; les *details* ne sont déchiffrés qu'à l'usage, et oubliés ensuite.

**Complément**, pour que la taille ne trahisse pas la catégorie :
- *overview* : jusqu'à un multiple de 256 octets (`0x80`, puis des `0x00`, comme SYNC.md § 3.5) ;
- *details* : jusqu'à `Padmé(max(n + 1, 1024))` octets, `n` étant la longueur du JSON, avec le même complément. Padmé (Nikitin et al., 2019) : pour `L ≥ 2`, `E = ⌊log₂ L⌋`, `S = ⌊log₂ E⌋ + 1`, `z = E − S`, et `Padmé(L)` = `L` arrondi au multiple supérieur de `2^z`. Il coûte au plus 12 %, et ne laisse fuir que O(log log n) bits de la taille.

La couche externe est en plus complétée à 256 octets, comme tout enregistrement.

**Limites :** *overview* de 4 Kio au plus, *details* de 96 Kio au plus, avant complément. Le contenu chiffré reste ainsi sous `maxPayloadBytes` (256 Kio). Au-delà, l'historique est raccourci (§ 9.5), puis l'élément ne part pas, comme une note trop grosse (SYNC.md § 2.10) : il reste sur ce Mac, signalé.

### 3.3 Empreinte d'un élément

L'empreinte d'une version d'un élément est :

```
SHA-256("matcha.vault.item.v1" ‖ L(itemKey) ‖ itemKey ‖ L(overview) ‖ overview ‖ L(details) ‖ details)
```

où les trois parties sont les octets décodés des `fields` (version et nonce compris), et `L(x)` la longueur de `x` sur 4 octets, gros-boutiste. Elle s'écrit en 64 caractères hexadécimaux minuscules. Les nonces étant aléatoires, chaque écriture d'un élément donne une nouvelle empreinte, même sans changement du contenu.

---

## 4. Le manifeste

**But :** qu'aucun élément ne puisse être caché, supprimé ou remis à une version ancienne sans être vu, ni par le serveur, ni par du code qui n'a que `dataKey`. Seul qui a la clé du coffre écrit le manifeste.

### 4.1 Racine et seaux

Le manifeste est une racine et seize seaux. Les éléments se répartissent dans les seaux selon le premier caractère hexadécimal de leur identifiant en minuscules (`0` à `f`). Écrire un élément ne réécrit que son seau et la racine, quelle que soit la taille du coffre.

**Seau** : enregistrement `vaultBucket` (`schema` 1), d'identifiant calculé `vault-bucket:<vaultId en minuscules>|<seau>` (SYNC.md § 2.2), `<seau>` étant `0` à `f`. Ses `fields` :

```json
{ "vaultId": "…", "keyVersion": 1, "bucket": "0", "manifest": "base 64" }
```

`manifest` = `0x01` ‖ Seal(manifestKey, seau complété à un multiple de 256 octets, `matcha.vault.bucket.v1|<vaultId en minuscules>|<seau>|<keyVersion>`). Le seau, une fois ouvert :

```json
{
  "deleted": { "5c1d9e7a-2b4f-4a6c-8e0d-1f3b5a7c9e2d": "2026-11-01T10:00:00.000Z" },
  "items": { "0f8c2a6e-5b3d-4e7a-9c1f-2d6b8e4a7c30": "cf16a0a0…" }
}
```

- `items` : pour chaque élément du seau, son identifiant en minuscules et l'empreinte de sa version en vigueur (§ 3.3).
- `deleted` : les **pierres tombales**, pour chaque élément supprimé définitivement, son identifiant et la date de sa suppression (horloge de la synchro). Une pierre tombale est gardée 180 jours, comme une suppression sur le serveur (SYNC.md § 4.9), puis retirée à la prochaine écriture du seau. Un élément est soit dans `items`, soit dans `deleted`, jamais dans les deux.

**Racine** : enregistrement `vaultManifest` (`schema` 1), d'identifiant calculé `vault-manifest:<vaultId en minuscules>`. Ses `fields` :

```json
{ "vaultId": "…", "keyVersion": 1, "manifest": "base 64" }
```

`manifest` = `0x01` ‖ Seal(manifestKey, racine complétée à un multiple de 256 octets, `matcha.vault.manifest.v1|<vaultId en minuscules>|<keyVersion>`). La racine, une fois ouverte :

```json
{
  "buckets": { "0": "68af6b3f…" },
  "clock": { "3f2a8c1e-6b0d-4c5e-9f1a-2b7d8e4c6a10": 1 },
  "modifiedAt": "2026-11-08T09:15:00.000Z",
  "modifiedBy": "3F2A8C1E-6B0D-4C5E-9F1A-2B7D8E4C6A10"
}
```

- `buckets` : pour chaque seau non vide, SHA-256 des octets décodés de son `manifest` (version et nonce compris), en hexadécimal minuscule. Un seau vide (ni élément, ni pierre tombale) **doit** être absent ; son enregistrement reste (jamais supprimé), et n'est plus vérifié.
- `clock` : un **vecteur de versions**. Pour chaque Mac qui a écrit le coffre, son identifiant (`deviceId`, en minuscules) et le nombre de ses écritures. Chaque écriture part du `clock` de la racine sur laquelle elle repose, et augmente de 1 l'entrée du Mac qui écrit. Une racine **succède** à une autre si chaque entrée de l'autre y est au moins aussi grande. Deux racines dont aucune ne succède à l'autre sont **divergentes** (§ 5.3).
- `modifiedAt`, `modifiedBy` : la date (horloge de la synchro, SYNC.md § 2.8) et le Mac de cette écriture. L'interface les affiche : « Dernière modification du coffre le … depuis … ». Contrairement à ceux de la couche externe, ils sont authentifiés par la clé du coffre.

À la réception, comme pour tout identifiant calculé, le client recalcule l'identifiant d'un seau ou d'une racine à partir de `vaultId` et `bucket`, et ignore un enregistrement qui ne correspond pas (SYNC.md § 2.2). La valeur de `<keyVersion>` des `aad` est celle de `fields.keyVersion`.

**Taille :** au plus 10 000 éléments et pierres tombales par coffre en v1. Un seau en porte en moyenne 625, soit moins de 100 Kio chiffrés. Au-delà, Matcha refuse d'ajouter un élément et le dit.

### 4.2 Ce que garantit le manifeste

- **Un élément montré est un élément cité.** Un élément n'est accepté que si la racine en vigueur, par son seau, le cite avec cette empreinte.
- **Pas de retour en arrière.** Chaque Mac garde la racine en vigueur, et n'accepte qu'une racine qui lui succède (§ 4.1). Dans une racine qui succède, un élément ne change que pour une version plus récente que la sienne (`modifiedAt`, puis `modifiedBy`, de l'*overview*, § 9.1).
- **Une suppression est écrite.** Un élément ne disparaît que si une pierre tombale le dit. Un élément qui n'est plus cité, sans pierre tombale, est une divergence.
- **Ce qui est altéré se répare** (§ 5.3) : un élément, un seau ou une racine que le serveur perd, remplace ou supprime revient depuis un Mac qui l'a vu.
- **Ce qu'il ne détecte pas :** un serveur qui fige tout le coffre pour un Mac, ou qui sert à un Mac neuf un état ancien mais complet. Le Mac neuf le prend tel quel. La date de la dernière modification affichée sert à le remarquer. L'état vérifié d'un Mac ne s'efface que sur demande (§ 5.5) : le serveur ne peut pas faire d'un Mac qui a vu le coffre un Mac neuf.

---

## 5. Synchro

### 5.1 Écriture : un lot atomique

Toute écriture du coffre part dans un **lot atomique** (`"atomic": true`, SYNC.md § 4.6) : les éléments changés, leurs seaux, et la racine. Le serveur applique tout ou rien.

- **Ce que porte la racine écrite :** `buckets` est celui de la racine en vigueur, où seuls les seaux réécrits changent. Un seau réécrit part du seau en vigueur, tel que la racine le cite (éléments en attente compris, § 5.2) : seules les entrées des éléments changés par ce Mac changent. `clock` est celui de la racine en vigueur, l'entrée de ce Mac augmentée de 1.
- **`baseVersion`** de chaque enregistrement : la version du serveur sur laquelle repose le changement, comme d'habitude, même si son contenu est illisible.
- **Appliqué :** ce que ce Mac a écrit devient son état vérifié, sans attendre de le relire.
- **Conflit :** si l'un des changements est en conflit, rien n'est écrit. Le client lit les changements (SYNC.md § 4.6), les vérifie (§ 5.2), fusionne ses modifications locales (§ 5.4), et recompose un nouveau lot, avec une nouvelle `Idempotency-Key`. Deux Mac qui écrivent en même temps entrent en conflit sur la racine, et le second recommence. Un conflit avec `"current": null` (suppression purgée sur le serveur) : le changement repart comme une création, ou, si c'était une suppression, il est retiré du lot.
- **Refusé :** `quota_exceeded` veut dire que le compte est plein : rien n'est perdu, le lot attend, et la synchro le dit. `payload_too_large` ou `invalid_payload` : l'élément en cause reste sur ce Mac, signalé, et le lot est recomposé sans lui (son seau garde l'empreinte en vigueur).
- **Sans réponse :** le lot est renvoyé tel quel, avec sa clé (SYNC.md § 4.6), même coffre verrouillé : il est déjà chiffré.
- **Réponse illisible :** une réponse à un envoi atomique est soit toute en `applied`, soit sans aucun `applied`. Autre chose est une réponse illisible (SYNC.md § 4.6), et le lot repart tel quel au cycle suivant.
- **Grandes écritures** (import, réconciliation : plus de 100 éléments, ou plus de 1 Mio) : les éléments partent d'abord en lots ordinaires, par tranches qui respectent `maxChangesPerPush` et `maxRequestBytes`. Tant qu'aucune racine ne les cite, aucun Mac ne les montre (§ 5.2, étape 6). Un dernier lot atomique porte ensuite les seaux et la racine (17 enregistrements au plus). S'il est en conflit, seuls les seaux et la racine sont recomposés, après lecture et fusion.
- Un lot du coffre ne porte que des enregistrements du coffre. Le moteur de la synchro n'envoie jamais un enregistrement du coffre lui-même : c'est le coffre qui le fait.
- Un serveur dont `features` ne contient pas `vault` n'a pas de coffre : Matcha le cache, sauf si ce Mac a déjà un état du coffre pour ce compte (§ 5.5), auquel cas c'est une erreur, signalée.

### 5.2 Lecture et vérification

Les enregistrements du coffre arrivent par la lecture habituelle (`/v1/sync/changes`). Le moteur de la synchro les garde tels quels (comme un type qu'il n'applique pas, SYNC.md § 2.2, avec sa règle « pas de retour en arrière », § 2.3), et les passe au coffre. Le coffre ne vérifie qu'**après une lecture complète** (`hasMore: false`) et **coffre ouvert** : le serveur écrit un lot atomique d'un coup (SYNC.md § 4.9), donc une lecture complète voit toujours une racine, ses seaux et ses éléments cohérents. Coffre verrouillé, la vérification attend le déverrouillage.

Chaque Mac garde son **état vérifié** : la racine en vigueur (chiffrée, et son `clock`), ses seaux, et pour chaque élément, la version qu'il en a (les trois parties chiffrées, l'empreinte, et le `modifiedAt` et le `modifiedBy` de son *overview*). C'est le cache local du coffre, chiffré (§ 7.4).

Vérifier, c'est comparer l'état du serveur (la dernière racine reçue, ses seaux, les éléments reçus) à l'état vérifié :

1. **La racine du serveur.** Absente, supprimée (`deleted: true`), indéchiffrable ou d'identifiant calculé faux :
   - si ce Mac a un état vérifié, c'est une altération : il la répare (§ 5.3) ;
   - sinon, sans aucun enregistrement du coffre, le coffre est vide ; avec des enregistrements du coffre, il n'est pas encore vérifiable : rien n'est montré, et le coffre le dit (« Le coffre attend un autre de vos Mac pour être vérifié »).
2. **Situer la racine.** Si elle succède à la racine en vigueur (§ 4.1), on continue. Si c'est la même (mêmes octets), rien n'a changé. Sinon (elle ne succède pas, ou elle a le même `clock` avec d'autres octets), c'est une **divergence** (§ 5.3).
3. **Les seaux** cités doivent avoir l'empreinte citée. Un seau qui ne l'a pas (absent, supprimé, autre contenu) : si ce Mac a le seau cité, il le répare ; sinon la racine est incomplète, l'état vérifié reste, et la lecture suivante réessaie.
4. **Chaque élément cité** :
   - la version du serveur a l'empreinte citée : elle est acceptée, sauf si ce Mac a une version différente de l'élément plus récente qu'elle (`modifiedAt`, puis `modifiedBy`, des *overviews*) : c'est alors un retour en arrière de cet élément, donc une divergence ;
   - elle ne l'a pas, elle manque, ou c'est une suppression : si ce Mac a la version citée, il la garde et la répare ; sinon l'élément est **en attente**. Sa dernière version vérifiée, s'il en a une, reste montrée, marquée « en attente ». Le coffre le signale (« N éléments du coffre n'ont pas pu être vérifiés ») et réessaie à la lecture suivante.
5. **Un élément de l'état vérifié que la racine ne cite plus :** si son seau a une pierre tombale pour lui, il est supprimé ; sinon, c'est une divergence.
6. **Un élément reçu que la racine ne cite pas** n'est pas montré, et n'est ni supprimé ni écrasé. C'est le cas d'un élément d'une grande écriture en cours (§ 5.1), ou d'un élément injecté.

Quand toutes les étapes passent, la racine du serveur devient la racine en vigueur, avec les versions acceptées. Un Mac sans état vérifié (Mac neuf, coffre jamais ouvert ici) accepte la racine du serveur telle quelle, et l'étape 4 sans comparaison.

### 5.3 Divergence et réparation

Une divergence, c'est un serveur qui montre un coffre plus ancien que celui que ce Mac a vu, ou qui en a perdu une partie. Elle arrive après la restauration d'une sauvegarde du serveur, face à un serveur malveillant, ou face à du code qui n'a que `dataKey`.

Le Mac **réconcilie** : il fusionne son état vérifié et l'état du serveur, sans rien perdre, puis l'écrit (§ 5.1, en grande écriture s'il le faut), avec pour `clock` le maximum des deux, entrée par entrée, et l'entrée de ce Mac augmentée de 1.
- **Un élément présent des deux côtés dans deux versions différentes :** la plus récente gagne (`modifiedAt`, puis `modifiedBy`, de l'*overview*), et chaque secret de l'autre version qui diffère passe dans son historique (§ 9.5), avec la date et le Mac de cette autre version.
- **Un élément présent d'un seul côté** est gardé, sauf si l'autre côté a une pierre tombale postérieure à sa version : la suppression est alors acceptée. Les pierres tombales des deux côtés sont gardées, la plus récente pour un même élément.

Comme après un curseur expiré (SYNC.md § 5), un élément supprimé ailleurs depuis plus de 180 jours peut revenir. C'est le prix pour ne jamais perdre un secret.

**Réparer** : un élément, un seau ou une racine dont la version du serveur n'est pas celle que cite la racine en vigueur (manquante, supprimée, remplacée), alors que ce Mac a la version citée, est renvoyé tel quel. Seule la couche externe change (`modifiedAt` nouvelle) ; la couche interne, donc l'empreinte, reste la même. La réparation part dans un lot du coffre à part, sans changer les seaux ni la racine (par exception au § 5.1), au plus une fois par jour et par enregistrement.

**Ce que voit l'utilisateur.** Une réconciliation ou une réparation est toujours signalée, dans le coffre et dans Réglages > Synchronisation : « Le coffre du serveur était incomplet ou plus ancien que celui de ce Mac (restauration du serveur, ou altération). Matcha l'a complété depuis ce Mac. » Même après un curseur expiré : c'est le serveur qui l'envoie, il ne doit pas pouvoir faire taire l'alerte.

### 5.4 Conflits : aucun secret n'est perdu

Quand une modification de ce Mac rencontre une autre version d'un même élément à l'envoi (conflit, § 5.1), le client fusionne **champ par champ**, par rapport à la version vérifiée dont les deux sont parties :

- un champ changé d'un seul côté prend cette valeur ;
- un champ changé des deux côtés différemment prend la valeur de la version la plus récente (`modifiedAt` puis `modifiedBy` de l'*overview*) ;
- **si c'est un secret** (§ 9.3), la valeur perdante passe dans l'historique de l'élément ;
- les listes dont chaque entrée a un `id` (identifiants, champs personnalisés, fichiers) se fusionnent entrée par entrée ; une entrée ajoutée d'un côté est gardée ; une entrée retirée d'un côté et modifiée de l'autre est gardée ;
- l'historique est l'union des deux, sans doublon (même secret, même valeur), trié par date, puis raccourci (§ 9.5) ;
- `tags` et `urls` se fusionnent comme un champ.

Sans version commune (réconciliation, § 5.3), c'est la règle du § 5.3 : la version la plus récente entière, et les secrets de l'autre dans l'historique.

Une modification l'emporte sur une suppression définitive qui ne lui est pas postérieure : l'élément est recréé, et sa pierre tombale retirée. La mise à la corbeille est un champ (`trashedAt`) : un élément modifié ici et mis à la corbeille ailleurs est les deux.

**Coffre verrouillé :** la fusion demande la couche interne. Un lot en conflit attend donc le déverrouillage. En général, il part bien avant : les modifications ne se font que coffre ouvert, et partent tout de suite.

### 5.5 L'état du coffre sur le Mac, et ce qui ne change pas

- **L'état vérifié n'est pas `SyncState`.** Il est gardé à part (§ 7.4), par serveur et par compte, et reste quand le serveur met fin à la session (`device_revoked`, `account_deleted`, jetons expirés), et après 60 jours sans reconnexion. Ces réponses ne sont pas authentifiées : sinon, le serveur pourrait faire d'un Mac qui a vu le coffre un Mac neuf, qui accepte n'importe quel état ancien. Il ne s'efface que sur demande de l'utilisateur de ce Mac : « Se déconnecter », « Supprimer le compte » depuis ce Mac, ou « Oublier le coffre de ce Mac ». Il est chiffré : rien de lisible ne reste.
- **Reconnexion** au même serveur et au même compte : l'état vérifié reprend. Une Session sans `wrappedVaultRootKey`, ou un serveur sans `vault`, alors que ce Mac a un état du coffre pour ce compte : c'est une erreur, signalée, et le coffre de ce Mac reste tel quel. Le client ne tire jamais de `vaultRootKey` après la création du compte.
- **Opérations sur tout le compte** (« Remplacer les données synchronisées », « Remplacer les données de ce Mac », première synchro, SYNC.md § 5) : elles **ne touchent jamais au coffre**. Le moteur traite ses enregistrements comme un type qu'il n'applique pas (SYNC.md § 2.2). Sinon, un Mac dont le coffre est verrouillé, ou n'a jamais été ouvert, effacerait le coffre de tous les autres.
- **Curseur expiré :** le moteur relit tout, et ne renvoie pas lui-même les enregistrements du coffre ; le coffre vérifie, et réconcilie s'il le faut (§ 5.3).
- **Horloge :** les règles de SYNC.md § 2.8. Un Mac à l'heure fausse donne de faux codes à usage unique : l'écran d'un code le dit dès 30 secondes d'écart avec l'en-tête `Date` du serveur.
- **Anciennes versions de Matcha :** un `vaultItem`, `vaultBucket` ou `vaultManifest` est pour elles un type inconnu, gardé sans y toucher (SYNC.md § 2.2).

### 5.6 Garde-fous

- Mettre plusieurs éléments à la corbeille en une fois, ou la vider, demande une confirmation qui dit combien.
- Une suppression définitive ne vise qu'un élément déjà dans la corbeille. La corbeille se vide seule au bout de 30 jours (§ 9.6), selon l'horloge de la synchro ; un Mac dont l'horloge est en avance de plus de 24 h sur le serveur ne vide rien (SYNC.md § 2.8).

---

## 6. Fichiers

Un fichier est rangé dans un élément (pièce jointe, ou élément de catégorie `file`). Il voyage comme une **suite de paquets** (SYNC.md § 4.7), sans rien changer au serveur.

- **Morceaux :** le fichier est découpé en morceaux de 16 Mio (16 777 216 octets), le dernier plus court ; un fichier vide fait un morceau vide. `n` morceaux, numérotés de 0 à `n − 1`.
- **Chiffrement :** chaque morceau est complété jusqu'à un multiple de 64 Kio (même complément que SYNC.md § 3.5), puis `0x01` ‖ Seal(fileKey, morceau complété, `matcha.vault.file.v1|<fileId en minuscules>|<index>|<n>`). Un morceau manquant, déplacé ou d'un autre fichier ne s'ouvre pas, comme dans la construction STREAM.
- **Paquets :** chaque morceau est un paquet (`PUT /v1/blobs/{id}`) d'identifiant aléatoire. 16 Mio de morceau donnent 16 Mio + 64 Kio + 29 octets, sous `maxBlobBytes` (21 Mio).
- **Dans les *details*** (§ 9.4) : `{ "id", "name", "type", "size", "sha256", "key", "chunks", "createdAt" }`. `id` est le `fileId` ; `type` le type MIME ; `size` la taille en octets ; `sha256` l'empreinte du fichier en clair, en hexadécimal minuscule ; `key` la `fileKey` en base 64 ; `chunks` les identifiants des paquets, dans l'ordre.
- **Taille :** 100 Mio au plus par fichier (R6), soit 7 morceaux.
- **Ordre d'envoi :** les paquets d'abord, puis l'élément qui les cite, avec le suivi des paquets en attente de SYNC.md § 4.7.
- **Paquets qui ne servent plus** (fichier retiré, élément supprimé définitivement) : ils ne sont supprimés que 30 jours après, si aucune version de l'état vérifié ne les cite encore. Une fusion ou une réconciliation peut en effet citer de nouveau un fichier retiré ailleurs.
- **Paquet introuvable :** un Mac qui a les morceaux (« Garder hors ligne », ou fichier ajouté depuis ce Mac) les renvoie, comme une réparation (§ 5.3).
- **Lecture :** à la demande. Le client télécharge les morceaux, vérifie leur ordre (l'`aad`), `size` et `sha256`. Un paquet introuvable : le fichier est montré « pas encore sur le serveur », avec « Réessayer ». « Garder hors ligne » garde les morceaux chiffrés sur ce Mac.
- **Sur le Mac :** un fichier ouvert est écrit en clair dans un dossier temporaire du conteneur de Matcha, effacé au verrouillage du coffre et à la fermeture de Matcha. « Enregistrer sous… » l'écrit où l'utilisateur le choisit.

Le serveur voit des paquets envoyés ensemble, juste avant un enregistrement, et leur taille arrondie à 64 Kio : il peut deviner qu'un fichier a été ajouté, et sa taille approximative (§ 11).

---

## 7. Sur le Mac

### 7.1 Déverrouiller par le mot de passe

PBKDF2 (avec le `salt` et les `iterations` du compte, gardés dans l'état du coffre), puis HKDF avec la Secret Key du trousseau, donnent `wrapKey`, qui ouvre `wrappedVaultRootKey`. Une copie de l'enveloppe est gardée dans l'état du coffre : le coffre s'ouvre hors ligne. Après un changement de mot de passe sur un autre Mac, ce Mac est déconnecté (SYNC.md § 3.6) ; la reconnexion apporte la nouvelle enveloppe.

À la connexion, à la création du compte et à la récupération, le client a `wrapKey` : il ouvre l'enveloppe tout de suite. Une enveloppe qui ne s'ouvre pas a été remplacée : le coffre est marqué illisible et le dit, au lieu d'afficher plus tard « Mot de passe incorrect ».

Un mot de passe faux ne s'ouvre pas : l'écran dit « Mot de passe incorrect ». Les essais ne passent pas par le serveur ; après 5 échecs, chaque essai attend 30 secondes de plus, jusqu'à 5 minutes.

### 7.2 Touch ID

- `vaultRootKey` est enveloppée par une clé P-256 de la Secure Enclave de ce Mac (`SecureEnclave.P256.KeyAgreement`), créée avec le contrôle d'accès `privateKeyUsage` et `biometryCurrentSet`. C'est la Secure Enclave qui exige l'empreinte, pas une vérification par `LAContext` qu'un code dans le processus pourrait sauter.
- **Enveloppe :** une clé P-256 éphémère `e` ; `secret` = ECDH(e, clé de la Secure Enclave) ; `k` = HKDF-SHA256(secret, sel = clé publique éphémère au format X9.63, info = "matcha.vault.touch-id.v1") ; enveloppe = clé publique éphémère ‖ Seal(k, vaultRootKey, "matcha.vault.touch-id.v1"). Pour l'ouvrir, la Secure Enclave refait l'ECDH avec la clé publique éphémère, après Touch ID.
- La clé de la Secure Enclave (sa `dataRepresentation`, inutilisable sur un autre Mac) et l'enveloppe sont dans le trousseau de Matcha (élément `vaultTouchID`, rangé par serveur et par compte comme les autres, SYNC.md § 3.8).
- **Activation :** proposée au premier déverrouillage par le mot de passe, sur un Mac qui a Touch ID ; réglable ensuite. Les empreintes changent : la clé ne sert plus, l'enveloppe est effacée, et le mot de passe est redemandé.
- Chaque déverrouillage prend un `LAContext` neuf, sans réutilisation d'une authentification récente (`touchIDAuthenticationAllowableReuseDuration` à 0).
- **Mot de passe redemandé** quand même : tous les 14 jours, après un redémarrage, et pour exporter (§ 10).
- Le coffre déverrouillé par Touch ID n'a jamais eu `wrapKey` : les opérations sensibles du compte (SYNC.md § 4.1) redemandent toujours le mot de passe.

### 7.3 Verrouillage

Le coffre se verrouille :
- après 15 minutes sans utilisation du coffre ni du remplissage (réglable : 1, 5, 15, 30 ou 60 minutes, ou « Jamais tant que le Mac est actif ») (R8) ;
- à la mise en veille, au verrouillage de l'écran, au changement d'utilisateur ;
- à la fermeture de Matcha ;
- par « Verrouiller le coffre » (menu, raccourci ⌃⌘L).

Verrouiller oublie `vaultRootKey`, les clés qui en dérivent, les *overviews* et les *details* déchiffrés, et efface les fichiers ouverts (§ 6). Le presse-papiers est vidé s'il contient encore un secret copié (§ 7.5).

### 7.4 Ce qui reste sur le disque

- **État du coffre**, un fichier par serveur et par compte, à côté de `SyncState` mais distinct de lui (§ 5.5) : les paramètres `kdf` du compte, la copie de `wrappedVaultRootKey`, l'état vérifié (§ 5.2 : racine, seaux, versions des éléments, chiffrés tels qu'ils sont arrivés), le lot en attente (chiffré), et les dates des réparations (§ 5.3). Rien n'y est en clair, sauf les identifiants des éléments, les dates et le `clock`.
- **Trousseau** : la Secret Key (SYNC.md § 3.8) et l'enveloppe Touch ID.
- **Jamais sur le disque :** `vaultRootKey` et ses dérivées, un *overview* ou des *details* déchiffrés, un mot de passe généré avant son enregistrement.

Avec le Mac et sa session ouverte, sans le mot de passe ni Touch ID, on peut seulement essayer des mots de passe contre la copie de l'enveloppe, au prix de PBKDF2 à chaque essai. C'est la limite admise de la discovery (§ 4).

### 7.5 Presse-papiers, écran, traces

- **Presse-papiers :** un secret copié porte les types `org.nspasteboard.ConcealedType` et `org.nspasteboard.TransientType`, que les gestionnaires de presse-papiers ignorent, et il est limité à ce Mac (`NSPasteboard.ContentsOptions.currentHostOnly`, pas de presse-papiers universel). Il est effacé au bout de 90 secondes s'il n'a pas changé, et au verrouillage.
- **Écran :** un secret est masqué par défaut, et ne s'affiche qu'à la demande (bouton, ou maintenir ⌥). Les fenêtres du coffre ont `sharingType = .none`. Ce réglage n'arrête pas toujours ScreenCaptureKit sous macOS 15 : c'est pourquoi le masquage par défaut est la vraie protection.
- **Traces :** aucun secret dans les journaux, les rapports de plantage, Spotlight, Quick Look, l'historique de la barre d'adresse, la fonction Annuler, ni les caches d'images. Les champs de saisie d'un secret sont des `NSSecureTextField`, ou équivalents.

### 7.6 Accès, et coffre désactivé

- **Accès :** le bouton du coffre, en tête des boutons des extensions (barre latérale, ou barre d'adresse sans barre latérale), ouvre sa fenêtre ; il est plein quand le coffre est ouvert. Aussi : Fenêtre > Coffre (⌥⌘P), et le lien « Ouvrir le coffre » de la liste du remplissage (§ 8.1).
- **Le coffre est facultatif :** Réglages > Coffre > « Utiliser le coffre sur ce Mac », activé par défaut, réglage de ce Mac seulement.
- **Désactivé**, le coffre est verrouillé et ne s'ouvre plus, ni par le mot de passe, ni par Touch ID, ni par un flux du compte. Son bouton, ses menus, le bouton du remplissage et la barre d'enregistrement disparaissent. Les pages n'ont plus le récepteur du script du remplissage (§ 8.3) : dès le chargement suivant, le script s'arrête aussitôt lancé, et une page déjà chargée ne peut plus rien envoyer.
- **Rien n'est effacé :** les éléments restent dans le compte et sur les autres Mac ; l'état vérifié, l'enveloppe et Touch ID restent sur ce Mac, et la synchro transporte toujours les enregistrements du coffre (§ 5.5). Réactivé, le coffre reprend la vérification là où ce Mac l'avait laissée. Pour effacer sa copie, il y a « Oublier le coffre sur ce Mac ».

---

## 8. Remplissage

### 8.1 Interface

- **L'interface du remplissage est native** (bouton dans la barre d'adresse, liste en surimpression de la fenêtre, raccourci ⌘\\), **jamais dans la page**. Aucun calque d'une page ne peut la cacher ni la superposer : c'est la parade au *clickjacking* d'extension (DEF CON 33, discovery § 8).
- **Rien n'est rempli d'office** : ni au chargement, ni à la frappe, seulement après une action de l'utilisateur dans l'interface native (choisir un identifiant, raccourci).
- Le bouton n'apparaît que sur une page qui a un champ de mot de passe, ou un identifiant enregistré pour ce site. La liste propose d'abord les identifiants du site, puis « Chercher dans le coffre ».
- Coffre verrouillé : le bouton propose de le déverrouiller (Touch ID ou mot de passe), puis remplit.
- Un code à usage unique est rempli dans un champ de code s'il y en a un, ou copié (§ 7.5) avec une notification.

### 8.2 Correspondance

Un identifiant a des adresses (`urls`) et une règle (`match`) :

| `match` | Une page correspond si |
| --- | --- |
| `site` (par défaut) | son hôte a le même **domaine enregistrable** que l'une des adresses |
| `host` | son hôte et son port sont ceux de l'une des adresses |
| `page` | son origine est celle de l'une des adresses, et son chemin est le même (sans requête ni fragment) |

- **Domaine enregistrable :** d'après la **liste publique des suffixes complète, section privée comprise** (`github.io`, `herokuapp.com`… sont des suffixes, donc `alice.github.io` et `bob.github.io` sont deux sites) (R7). Elle est embarquée dans Matcha et mise à jour avec l'app. Algorithme de la liste (règles, jokers `*`, exceptions `!`), sur l'hôte en minuscules, en ASCII (punycode), sans point final. Une adresse IP, `localhost`, ou un hôte qui est lui-même un suffixe public, est pris en entier. Ce n'est pas `SiteDomain` (SYNC.md § 2.7), qui reste pour les zooms et les notes.
- **Ports et chemins :** pour `host` et `page`, le port par défaut du schéma (80, 443) est retiré, le `/` final du chemin ignoré, et le chemin comparé tel qu'il est encodé.
- **Schéma :** seulement `http` et `https`. Un identifiant enregistré en `https` n'est **jamais** rempli sur une page `http`. Un identifiant enregistré en `http` est rempli sur `http` ou `https`, avec un avertissement sur `http`.
- **Cadres :** un cadre n'est rempli que si **sa propre origine** correspond à l'identifiant. Pour un cadre d'une autre origine que la page, la liste le dit : « Dans un cadre de example.com ».
- **Adresses :** une adresse saisie sans schéma prend `https://`. Elle est gardée telle quelle dans `urls`, et comparée selon la règle à chaque fois : comme pour les zooms, la liste des suffixes peut changer d'une version à l'autre sans rien réécrire.

Exemples, pour un identifiant enregistré sur `https://accounts.example.co.uk/login`, en `site` :

| Page | Correspond |
| --- | --- |
| `https://www.example.co.uk/` | oui |
| `https://example.co.uk:8443/` | oui |
| `http://example.co.uk/` | non (enregistré en `https`) |
| `https://example.com/` | non |
| `https://alice.github.io/` pour un identifiant sur `https://bob.github.io/` | non |

### 8.3 Scripts

- Les scripts qui trouvent les formulaires, remplissent les champs et repèrent les envois tournent dans un `WKContentWorld` à part, que la page ne voit pas : elle ne peut ni les appeler, ni lire leurs variables, ni remplacer leurs fonctions.
- Ils ne reçoivent un secret qu'au moment de remplir, pour un cadre précis (`WKFrameInfo`), après la vérification de son origine par Matcha. Cette origine est celle de `WKFrameInfo.securityOrigin` ; une origine opaque (`null`) n'est jamais remplie. Le cadre a pu changer de page entre-temps : Matcha passe aussi l'origine attendue au script, qui revérifie `location.origin` juste avant de remplir, et ne remplit rien sinon.
- Ils remplissent comme une saisie : `input` et `change` sont envoyés, pour que les sites modernes voient la valeur.
- Ils ne s'exécutent jamais dans une fenêtre privée sans action de l'utilisateur, ni sur une page qui n'est pas `http` ou `https`.

### 8.4 Enregistrer

- Après l'envoi d'un formulaire de connexion, ou d'un formulaire d'inscription ou de changement de mot de passe, une **barre native** propose « Enregistrer le mot de passe pour example.com ? » (Enregistrer, Jamais pour ce site, Plus tard). Pour un identifiant déjà enregistré dont le mot de passe a changé : « Mettre à jour le mot de passe ? ». L'ancien passe dans l'historique (§ 9.5).
- Un formulaire d'inscription propose un mot de passe généré (§ 9.7), dans l'interface native.
- **Fenêtres privées :** le remplissage y marche, mais rien n'y est proposé à l'enregistrement sans le demander.
- « Jamais pour ce site » est gardé sur ce Mac (réglage local, par site au sens du § 8.2).

---

## 9. Modèle des éléments

**Aligné sur le Credential Exchange Format de la FIDO Alliance (CXF)** : un élément porte une liste de *credentials*, de types CXF quand ils existent (`basic-auth`, `totp`, `credit-card`, `ssh-key`, `passport`, `drivers-license`, `identity-document`, `address`, `person-name`…). L'import et l'export CXF en découlent (§ 10). Les valeurs sont écrites comme des textes simples, et non comme les `EditableField` de CXF.

### 9.1 *Overview*

```json
{
  "category": "login",
  "title": "GitHub",
  "subtitle": "jeremy",
  "favorite": false,
  "tags": ["dev"],
  "urls": ["https://github.com/login"],
  "match": "site",
  "trashedAt": null,
  "modifiedAt": "2026-11-08T09:15:00.000Z",
  "modifiedBy": "3F2A8C1E-6B0D-4C5E-9F1A-2B7D8E4C6A10"
}
```

- `category` : § 9.3. `title` : jamais vide à l'écriture (« Sans titre »). `subtitle` : ce que la liste affiche sous le titre (nom d'utilisateur, quatre derniers chiffres d'une carte, empreinte d'une clé SSH) ; jamais un secret.
- `urls` et `match` : pour les identifiants (§ 8.2) ; `[]` et `site` sinon.
- `tags` : textes libres, sans doublon. `favorite` : booléen.
- `trashedAt` : la date de mise à la corbeille, ou `null` (§ 9.6).
- `modifiedAt`, `modifiedBy` : la dernière modification du contenu (horloge de la synchro, SYNC.md § 2.8), authentifiée, qui départage les conflits (§ 5.4).

### 9.2 *Details*

```json
{
  "credentials": [
    { "id": "C1", "type": "basic-auth", "username": "jeremy", "password": "…" },
    { "id": "C2", "type": "totp", "secret": "JBSWY3DPEHPK3PXP", "algorithm": "sha1", "digits": 6, "period": 30, "issuer": "GitHub" }
  ],
  "fields": [{ "id": "F1", "label": "PIN", "type": "concealed", "value": "…" }],
  "notes": "",
  "files": [],
  "history": [{ "field": "C1.password", "value": "…", "at": "2026-10-17T08:30:12.345Z", "by": "3F2A8C1E-…" }],
  "createdAt": "2026-11-08T09:15:00.000Z"
}
```

- `id` d'un credential, d'un champ ou d'un fichier : un texte court, unique dans l'élément, gardé toute sa vie (la fusion s'en sert, § 5.4).
- `fields` : champs personnalisés, `type` = `text`, `concealed` (secret), `url`, `email`, `phone`, `date` (`AAAA-MM-JJ`) ou `totp` (comme un credential `totp`, dans `value` sous forme `otpauth://`).
- `notes` : texte libre, jusqu'à 64 Kio.
- `files` : § 6.
- `history` : § 9.5.

### 9.3 Catégories de la v1

| `category` | Credentials | Remarques |
| --- | --- | --- |
| `login` | `basic-auth` (`username`, `password`), `totp` facultatif | remplissage (§ 8) ; un code rangé avec son mot de passe réduit la 2FA à un facteur : l'écran le dit |
| `note` | aucun | `notes` |
| `card` | `credit-card` (`number`, `fullName`, `cardType`, `verificationNumber`, `pin`, `expiryDate` `AAAA-MM`, `validFrom` `AAAA-MM`) | `subtitle` : `•••• 4242` |
| `identity` | `person-name` (`given`, `given2`, `surname`, `title`), `address` (`streetAddress`, `postalCode`, `city`, `territory`, `country`, `tel`) ; champs `email`, `phone` | remplissage des formulaires plus tard |
| `document` | `passport`, `drivers-license` ou `identity-document` (`documentNumber`, `fullName`, `birthDate`, `issueDate`, `expiryDate`, `issuingCountry`, `issuingAuthority`) | documents d'identité |
| `sshKey` | `ssh-key` (`keyType`, `privateKey`, `publicKey`, `fingerprint`, `keyComment`) | § 9.4 |
| `file` | aucun | un ou plusieurs fichiers (§ 6) |
| `custom` | aucun | `fields` seulement : tout ce qui manque (compte bancaire, Wi-Fi, licence, serveur…) |

Les secrets de chaque catégorie (§ 9.5) : `password`, `secret` d'un `totp`, `number`, `verificationNumber` et `pin` d'une carte, `privateKey`, `documentNumber`, la `value` d'un champ `concealed` ou `totp`, et `notes`.

**Catégorie inconnue, `schema` plus récent, ou credential d'un type inconnu :** l'élément est montré avec son titre, en lecture seule (« Mettez Matcha à jour pour modifier cet élément »). Il peut aller à la corbeille : seul `trashedAt` change, et les champs inconnus sont gardés et renvoyés tels quels (SYNC.md § 2.2). Les champs inconnus d'un élément connu sont gardés de la même façon.

**Valeurs illisibles :** une couche interne indéchiffrable, ou un JSON illisible, met l'élément en attente (§ 5.2). Un champ du mauvais type prend sa valeur par défaut (`""`, `false`, `[]`, `null`), comme SYNC.md § 2.9 ; une `category` illisible vaut `custom`, en lecture seule.

### 9.4 Clés SSH

- `privateKey` : la clé privée au format OpenSSH (`-----BEGIN OPENSSH PRIVATE KEY-----`), sans phrase de passe : c'est le coffre qui la protège.
- `publicKey` : la ligne de `authorized_keys` (`ssh-ed25519 AAAA… commentaire`). `fingerprint` : `SHA256:` suivi de l'empreinte en base 64 sans complément, comme `ssh-keygen -l`. `keyComment` : le commentaire.
- `keyType` : `ssh-ed25519` (par défaut), `ecdsa-sha2-nistp256`, `ecdsa-sha2-nistp384`, ou `ssh-rsa` (3 072 ou 4 096 bits ; 2 048 bits au moins à l'import).
- **Générer :** Ed25519 (`Curve25519.Signing`), ECDSA P-256 et P-384 (CryptoKit), RSA (Security).
- **Importer :** le format OpenSSH, chiffré ou non (`bcrypt` + `aes256-ctr`, la phrase de passe est demandée et la clé déchiffrée à l'import), et PKCS#8, PKCS#1 ou SEC1 en PEM, non chiffrés. `bcrypt_pbkdf` (sur Blowfish) n'est pas dans les frameworks d'Apple : il est écrit dans Matcha, et vérifié par des vecteurs connus.
- **Copier la clé publique**, « Enregistrer la clé privée… » (fichier en `0600`). L'agent SSH viendra avec le client annexe (discovery § 9).

### 9.5 Historique

- Quand un secret change (modification ici, ou valeur perdante d'une fusion, § 5.4), l'ancienne valeur passe dans `history` : `{ "field", "value", "at", "by" }`. `field` désigne le secret : `<id du credential>.<nom>` (`C1.password`), `fields.<id du champ>`, ou `notes`. `at` et `by` : la date et le Mac de la valeur remplacée.
- Au plus 10 valeurs par secret, et 50 en tout ; les plus anciennes partent d'abord. Si les *details* dépassent leur limite (§ 3.2), les plus anciennes valeurs partent jusqu'à ce qu'ils y tiennent.
- L'historique s'affiche dans l'élément (« Anciens mots de passe »), masqué, et chaque valeur peut être copiée ou effacée. **Limite :** une valeur effacée ici peut revenir avec la fusion d'une modification faite en même temps ailleurs (union, § 5.4).
- Le serveur ne garde que la dernière version de chaque enregistrement (SYNC.md § 4.9) : l'historique est le seul.

### 9.6 Corbeille

- Supprimer un élément, c'est le mettre à la corbeille : `trashedAt` reçoit la date. L'élément reste dans le manifeste ; il ne remplit plus rien et n'apparaît plus que dans la corbeille, d'où on peut le remettre.
- La suppression définitive (`deleted: true`, et l'élément retiré de son seau) part à la demande (« Supprimer définitivement », « Vider la corbeille »), ou au bout de 30 jours, envoyée par le premier Mac déverrouillé qui le voit. Plusieurs Mac qui l'envoient en même temps entrent en conflit : le second relit, et n'a plus rien à faire.

### 9.7 Générateur, codes, analyse

- **Générateur :**
  - mot de passe : de 8 à 100 caractères (20 par défaut), minuscules, majuscules, chiffres et symboles (`!#$%&()*+,-./:;<=>?@[]^_{|}~`), au moins un de chaque ensemble choisi, option « sans caractères ambigus » (`0O1lI|`) ;
  - phrase de passe : de 3 à 12 mots (5 par défaut) de la grande liste de l'EFF (7 776 mots), séparés par `-` (réglable), option majuscule initiale et chiffre ;
  - tirage uniforme par rejet, sur le générateur du système ;
  - la jauge est celle de `SyncPasswordStrength`, avec l'entropie en bits.
- **Codes à usage unique** (RFC 6238, sur RFC 4226) : `secret` en base 32 (RFC 4648, sans complément, insensible à la casse, espaces ignorés), `algorithm` = `sha1`, `sha256` ou `sha512`, `digits` de 6 à 8, `period` de 15 à 120 secondes (30 par défaut). Une adresse `otpauth://totp/…`, ou un code QR à l'écran (capture d'une zone, sur demande), se lit à la saisie. Le code est calculé à l'heure du Mac.
- **Analyse :** sur le Mac, coffre ouvert, jamais envoyée : mots de passe faibles (`SyncPasswordStrength`), réutilisés entre plusieurs identifiants, identifiants en `http`. La vérification des fuites (Have I Been Pwned en k-anonymat) viendra plus tard, en option.

---

## 10. Import et export

- **Import :** CSV de Chrome (`name,url,username,password,note`), de Safari (`Title,URL,Username,Password,Notes,OTPAuth`) et de Firefox (`url,username,password,httpRealm,formActionOrigin,guid,timeCreated,timeLastUsed,timePasswordChanged`) ; JSON non chiffré de Bitwarden ; 1PUX de 1Password ; CXF. L'écran montre ce qui sera importé, les doublons (même site, même nom d'utilisateur, même mot de passe) et ce qui ne peut pas l'être. Tout entre dans un seul lot atomique par tranche de 200 éléments, et sous un tag `Import <date>`. Le fichier n'est jamais copié par Matcha, et l'écran rappelle de le supprimer. L'import s'ouvre depuis Fichier > Importer depuis > Fichier de mots de passe…, depuis la fenêtre du coffre (menu Importer et exporter), et quand le coffre est vide.
- **Export :** CXF, après le mot de passe maître (pas Touch ID).
  - **Chiffré** (par défaut) : un fichier `.matchavault`. C'est le seul fichier sans Secret Key : son mot de passe d'export est par défaut une phrase de passe générée de 6 mots (§ 9.7, environ 77 bits), montrée une fois ; l'utilisateur peut en choisir un autre, de 12 caractères au moins, avec la jauge. Format : en-tête = `MATCHAVAULT1` ‖ sel de 16 octets ‖ itérations sur 4 octets (gros-boutiste) ; puis `0x01` ‖ Seal(PBKDF2-SHA256(mot de passe NFKD, sel, itérations), JSON complété à un multiple de 4 Kio, aad = "matcha.vault.export.v1|" ‖ en-tête). À l'écriture, 1 000 000 d'itérations ; à la lecture, de 600 000 à 10 000 000, comme SYNC.md § 3.1, sinon le fichier est refusé.
  - **En clair**, seulement après un avertissement qui dit que le fichier contient tous les secrets.
  - Les fichiers (§ 6) ne sont pas exportés en v1 : l'écran le dit, et invite à les enregistrer depuis leur élément.

---

## 11. Ce que voit le serveur

**Il voit :** ce qu'il voit pour la synchro (SYNC.md § 3.9), plus les deux enveloppes de `vaultRootKey`. Les éléments, les seaux et la racine sont des enregistrements comme les autres. Il ne sait pas lesquels appartiennent au coffre.

**Fuites admises :**
- le nombre total d'enregistrements et leurs tailles arrondies ;
- les lots atomiques : ils ne portent que des enregistrements du coffre, ce qui dit au serveur lesquels sont ceux du coffre, et leur taille (un élément, son seau, la racine) laisse deviner une écriture d'un élément ;
- les horaires : un élément enregistré au moment où l'utilisateur s'inscrit sur un site ;
- des paquets envoyés ensemble, ce qui laisse deviner un fichier et sa taille approximative (§ 6).

**Modèle de menace :** celui de la discovery (§ 4). En particulier :

| Adversaire | Il ne peut pas |
| --- | --- |
| Serveur malveillant, ou fuite de sa base | lire, modifier ou injecter un élément ; deviner le mot de passe (Secret Key) ; cacher, supprimer ou remettre en arrière un élément sans qu'un Mac qui l'a vu le voie et le répare (§ 5.3) |
| Code qui a `dataKey` (Matcha détourné, coffre verrouillé) | lire un titre, une adresse ou un secret ; supprimer ou modifier un élément sans être vu (§ 5.3) |
| Mac volé, session fermée (FileVault) | ouvrir le coffre |
| Mac volé, session ouverte, coffre verrouillé | ouvrir le coffre sans le mot de passe : PBKDF2 contre la copie de l'enveloppe (§ 7.4) |
| Page web malveillante | obtenir un remplissage pour une autre origine, ou par un clic piégé (§ 8) |

**Limites admises :**
- un Mac compromis coffre ouvert perd ce qui est ouvert, `vaultRootKey` comprise. Elle ne change jamais en v1 : après le vol d'un Mac ouvert, une récupération ne protège pas les éléments écrits ensuite contre qui a copié cette clé. Sa rotation viendra plus tard (`keyVersion`, § 2.2) ;
- une mise à jour malveillante de Matcha perd tout ;
- un Mac neuf accepte l'état que le serveur lui montre (§ 4.2) ;
- un mot de passe tapé à la main sur un faux site n'est protégé que par le remplissage par origine.

---

## 12. Vecteurs de test

Calculés par une implémentation indépendante (Python `cryptography`) ; un client doit les retrouver (`VaultCryptoTests`, et le client de test Go). Entrées : celles de SYNC.md § 3.10 (donc les mêmes `wrapKey`, `recoveryWrapKey` et `idKey`), plus :
- `vaultRootKey` = octets 208 à 239 ;
- `itemKey` = 32 octets valant `0x11` ; `fileKey` = 32 octets valant `0x22` ;
- nonces = 12 octets valant `0xA1` (enveloppe), `0xA2` (enveloppe de secours), `0xA3` (`itemKey`), `0xA4` (*overview*), `0xA5` (*details*), `0xA6` (seau), `0xA7` (racine), `0xA8` (morceau de fichier).

| Valeur | Résultat |
| --- | --- |
| `wrappedVaultRootKey` (base 64) | `oaGhoaGhoaGhoaGhEOxauvt9iwApCVrUiHK62nCeXEyW8TvEbI8xQnr0mK1RKoY25LICmBnLMoj46sw+` |
| `recoveryWrappedVaultRootKey` (base 64) | `oqKioqKioqKioqKi/O/doW8QrILBadMk0oZpdE7H+MfGYVN1ydlMb/xNojP0DQpiSgDsZGGd4pyRvBoi` |
| `vaultKey` | `8261db72fb9ebf224e624f04aa9d445a64aaac04ac53f8e9668f3c67e93dd6de` |
| `itemWrapKey` | `a66570248ec1e58cf77e60053367b6a8163f92d4960ad6aa18e5f3baf2fffbce` |
| `manifestKey` | `2358b6d07fdec265ec600aff1e6a4923395e279705b34b348ece7c92a9dd69af` |
| `vaultId` | `02AFD736-3BA5-8284-BE72-68DCE1F17671` |
| racine (`vault-manifest:<vaultId>`) | `12FFDC1F-CB5A-851F-A0E8-FF1D5482C62D` |
| seau 0 (`vault-bucket:<vaultId>\|0`) | `F16781CF-1918-8D31-9C78-8D9F12E85B96` |

**Élément** : identifiant `0F8C2A6E-5B3D-4E7A-9C1F-2D6B8E4A7C30` (seau 0), `keyVersion` 1. *Overview* (249 octets, complété à 256) :

```json
{"category":"login","favorite":false,"match":"site","modifiedAt":"2026-11-08T09:15:00.000Z","modifiedBy":"3F2A8C1E-6B0D-4C5E-9F1A-2B7D8E4C6A10","subtitle":"jeremy","tags":["dev"],"title":"GitHub","trashedAt":null,"urls":["https://github.com/login"]}
```

*Details* (311 octets, complétés à 1 024) :

```json
{"createdAt":"2026-11-08T09:15:00.000Z","credentials":[{"id":"C1","password":"correct horse battery staple","type":"basic-auth","username":"jeremy"},{"algorithm":"sha1","digits":6,"id":"C2","issuer":"GitHub","period":30,"secret":"JBSWY3DPEHPK3PXP","type":"totp"}],"fields":[],"files":[],"history":[],"notes":""}
```

| Valeur | Résultat |
| --- | --- |
| `aad` de `itemKey` | `matcha.vault.item-key.v1\|02afd736-3ba5-8284-be72-68dce1f17671\|0f8c2a6e-5b3d-4e7a-9c1f-2d6b8e4a7c30\|1` |
| `itemKey` (base 64) | `AaOjo6Ojo6Ojo6Ojo5B41srfBJztsNM+eH4hy0i/PDDMhFs/U9zzWqtC7AOpTsibBHGeJtXBEqCrGI9kCQ==` |
| `details` (1 053 octets), SHA-256 | `dd6959378ff61c404dc6a03ea42870e400f294607ca0876abffaae8d1d56c555` |
| empreinte de l'élément (§ 3.3) | `cf16a0a026abc5e2824ef98804918c1ed1951800859bbc7a1d835978dc240665` |

`overview` (285 octets, base 64) :

```
AaSkpKSkpKSkpKSkpJTImjZEfuEDI9mXbunaMltEPJY2/aq0qzHS6ZE5EpB5Q2wq82wtAGXM4xTXcvHg9l07GEsCt6sC8BXF
wvN4tSZeeDuBoLQ8fvPnM1Tc/UTmgA6nfkatLW3hLhkMFt/XDgGRLqBrIYKcoHf6kst5tAm6nuaXUKBpr3Q8Z9LyYT9JeHjx
9cvdR3YemdhgLjARp3tOF2wYW/uu0y+lJE7c7qPdE4tKt2wY2lCMvSOd8U71b/KS1+akOg/MIJ+gmJk07Qfo6fdN1pXZ/Aov
OL4AEgzh9ihxLcS9Nnmb7RZpCmGwWr6GYn1gXXDTiQ0HiqgZJdOtwQZqOmYssXhYg+LMl7L3N/o6U3GsFjmumIOrTxaA
```

**Seau 0**, une fois ouvert (un élément, et une pierre tombale) :

```json
{"deleted":{"5c1d9e7a-2b4f-4a6c-8e0d-1f3b5a7c9e2d":"2026-11-01T10:00:00.000Z"},"items":{"0f8c2a6e-5b3d-4e7a-9c1f-2d6b8e4a7c30":"cf16a0a026abc5e2824ef98804918c1ed1951800859bbc7a1d835978dc240665"}}
```

`manifest` du seau (base 64), et son empreinte `68af6b3fc517a14fb9b0ec35e8407651e3cc513d97f064ec9441af472ab9fafc` :

```
Aaampqampqampqampv20zjnk5BEolTd57Py/IIEQJqx0M3d5dmamU4DybL4A5lCeqpXZJONfTELyEBiWAkBn9VQwbt/kYEeA
6Jc0lwcOO/eSWe2ZI0JunA5BatHVlllBgCc/UVpnH20pgxICeSRNLNCq9ECAromaQzHzF9wE2+a8Rh0BmDwy/eVyYaiZNLgt
lh7cFmFITBUr3nT5/D6bljDXEvrHEigAJOBpo/UqbnXkK1fFrLeRZNpGV/Kw2wIATMOGTWt9/Z5KnxydIhHnzkooGSGteNLx
ZDTRSIndyAHcQS5jwxvnQaigJcm7BeSmbB1IxzW5umpQEv37TEbKDYYi8OGa8QNY6inHbli208UZUFkxmhelm/NDWh9j
```

**Racine**, une fois ouverte :

```json
{"buckets":{"0":"68af6b3fc517a14fb9b0ec35e8407651e3cc513d97f064ec9441af472ab9fafc"},"clock":{"3f2a8c1e-6b0d-4c5e-9f1a-2b7d8e4c6a10":1},"modifiedAt":"2026-11-08T09:15:00.000Z","modifiedBy":"3F2A8C1E-6B0D-4C5E-9F1A-2B7D8E4C6A10"}
```

`manifest` de la racine (base 64) :

```
Aaenp6enp6enp6enpzr7aWaj5an1/5ZxtXyOkuFsZC0f4+urNSyZxd3QrkD8f401pDR8ZBdIP7KNV5H5LP+0LH2Zs4EdKz9U
Zl/t1liEzGSJG8Y3FBj2//d/K+/awVYJmLRyFwVD0MRhtidF7oF7EpDnr1Yfuo5HmveAqU4TgyDz0Z60uCkvdbvrQsaXK5DA
qDUBc/to98qdN2aC8+eUj8Jmh0yietWJtCH+UfYWPQBHcWtC+vosjvYL7OD7FGYJKbPAZIHUvHx/QYPV16yPLVs3oIR6kzYz
C3xVO8zk9QjDfAezGTLZFAJEZj5aJ3kEQf4Q604UBLNmOE0rvUOeHGnqrnQdGKx+Rn2X4XZSftBt0+GEYJkx1oWGsw8f
```

**Complément des *details*** (avec l'octet `0x80` et le plancher de 1 Kio) :

| JSON de | Complété à |
| --- | --- |
| 0 à 1 023 octets | 1 024 |
| 1 024 octets | 1 088 |
| 1 500 octets | 1 536 |
| 5 000 octets | 5 120 |
| 20 000 octets | 20 480 |
| 70 000 octets | 71 680 |

**Morceau de fichier** : `fileId` `3B9E1C4A-7D2F-4E8B-A6C0-5F1D2E3A4B59`, un seul morceau (index 0, `n` = 1), contenu = `Matcha vault file\n` répété 100 fois (1 800 octets, SHA-256 `5b499b55ca0b253aab15898936a55e6d66fd062726a394455c682c803d768d43`), complété à 65 536 octets. Morceau chiffré : 65 565 octets, SHA-256 `6bcc43e4c2969c2db20d701bd625b3637ac1debd9483c8ee44007d5d5641296d`.

**Codes à usage unique :** les vecteurs de la RFC 6238 (annexe B), avec les secrets ASCII `12345678901234567890` (SHA-1), `12345678901234567890123456789012` (SHA-256) et `1234567890123456789012345678901234567890123456789012345678901234` (SHA-512), 8 chiffres, 30 secondes. Par exemple, à `T` = 59 : `94287082`, `46119246`, `90693936`.

---

## 13. Décisions

Les décisions D1 à D9 sont celles de la discovery (§ 10). Tranchées le 2026-11-08, comme recommandé :

| # | Question | Décision |
| --- | --- | --- |
| R2 | Manifeste et `push` atomique en v1 | oui (§ 4, § 5.1) |
| R3 | Un coffre ou plusieurs | un seul, avec tags et favoris ; le format en permet d'autres (§ 2.3) |
| R4 | Catégories | identifiant (avec code), note, carte, identité, document, clé SSH, fichier, personnalisé (§ 9.3) |
| R5 | Historique | dans les *details*, borné ; corbeille de 30 jours (§ 9.5, § 9.6) |
| R6 | Taille d'un fichier | 100 Mio, téléchargement à la demande (§ 6) |
| R7 | Correspondance des sites | liste publique des suffixes complète, embarquée (§ 8.2) |
| R8 | Verrouillage | 15 minutes, veille, écran verrouillé ; Touch ID proposé au premier déverrouillage (§ 7.2, § 7.3) |

**Écarts avec la discovery**, et pourquoi :
- **`vaultRootKey` tirée à la création du compte**, et non à l'activation du coffre (`PUT /v1/account/vault`, abandonnée) : l'enveloppe de secours demande la clé de secours, que le client n'a qu'à la création du compte (et à la récupération). Activer le coffre plus tard aurait obligé à ressortir le kit.
- **`vaultKey` dérivée pour le coffre personnel**, et non tirée et enveloppée dans un enregistrement `vaultKey` : rien à créer, aucune course entre deux Mac, aucun enregistrement de clé qu'un serveur pourrait remplacer. Les coffres partagés auront des clés tirées (§ 2.3).
- **Manifeste en seize seaux sous une racine,** sans HMAC à part : le chiffrement authentifié de la racine et des seaux suffit.
- **Réconciliation et réparation** (§ 5.3) : un Mac qui a vu un élément, un seau ou une racine le renvoie si le serveur le perd. La discovery ne disait que « détecter ».

**Revue adversariale du 2026-11-08** (relecture indépendante de la première version de cette spec), corrigée ici :
- un simple compteur dans la racine laissait passer un retour en arrière d'un élément sous une racine de compteur plus grand : remplacé par un **vecteur de versions** (`clock`), et un élément ne change que pour une version plus récente (§ 4.1, § 5.2) ;
- une suppression n'était authentifiée que par une absence et la corbeille de ce Mac : remplacée par des **pierres tombales** dans les seaux (§ 4.1) ;
- un seau ou une racine altérés figeaient le coffre : ils se **réparent** comme les éléments, et une racine supprimée n'est jamais un coffre vide (§ 5.2, § 5.3) ;
- l'état vérifié suivait `SyncState`, qu'efface un `account_deleted` non authentifié : il est **gardé à part**, et ne s'efface que sur demande (§ 5.5) ;
- une réconciliation après un curseur expiré n'était pas signalée : elle l'est toujours (§ 5.3) ;
- les paquets d'un fichier retiré étaient supprimés tout de suite : **30 jours** après (§ 6) ;
- les lots atomiques pouvaient dépasser les limites : les **grandes écritures** passent en deux temps (§ 5.1) ;
- l'export : phrase de passe générée par défaut, itérations bornées à la lecture, en-tête authentifié (§ 10) ;
- le remplissage revérifie l'origine du cadre dans le script (§ 8.3) ; l'enveloppe est ouverte dès la connexion (§ 7.1) ;
- `vaultId` ne dépend plus d'`accountKey` (§ 2.3) ; le quota d'un envoi atomique se compte sur son solde net (SYNC.md § 4.6).
