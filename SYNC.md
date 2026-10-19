# SYNC.md — synchronisation de Matcha

Spécification de la synchronisation : données échangées (profils, Spaces, dossiers, onglets, extensions, réglages et décisions de site), chiffrement par mot de passe maître et API des serveurs. **Version 1, brouillon** jusqu'à la tâche 7.4 de `PLAN.md` ; révisée le 2026-10-18 (onglets du jour, extensions, réglages et sites), puis le 2026-10-19 (identifiants calculés par HMAC, formats figés, horloge, cas du moteur, session et appareils, lot S d'`AUDIT.md`). Elle fait foi pour le client (`Matcha/Services/Sync/`) comme pour le serveur de référence (en Go, dépôt `github.com/matchabrowser/syncserver`, tâche 7.8).

---

## 1. Principes

Trois modes, au choix dans Réglages > Synchronisation :

| Mode | Stockage | Protection | Disponible |
| --- | --- | --- | --- |
| iCloud | base privée CloudKit du compte Apple | chiffrement de CloudKit (`encryptedValues`), sans mot de passe maître | non : reporté (`FEATURES.md`), § 6 gardé pour la reprise |
| Serveur privé | serveur installé par l'utilisateur (API v1, § 4) | chiffrement de bout en bout par mot de passe maître (§ 3) | oui |
| Serveur public | serveur hébergé par Matcha, même API | idem | non : option grisée « Bientôt disponible » |

- **Un moteur, trois transports.** Contenu, conflits et suppressions suivent les mêmes règles dans les trois modes. Seul le transport change.
- **Les serveurs ne savent rien.** Ils ne reçoivent que des contenus chiffrés sur le Mac. Ils ne peuvent pas les lire, ni les modifier sans que le client s'en aperçoive. Sans le mot de passe maître ou la clé de secours, personne ne peut récupérer les données, pas même l'administrateur du serveur.
- **Une seule API.** Le serveur public est un serveur privé ouvert à tous.

---

## 2. Données

### 2.1 Ce qui est synchronisé

| Type | Source sur le Mac | Champs (`fields`) |
| --- | --- | --- |
| `profile` | `ProfileRecord` | `name`, `order` |
| `space` | `SpaceRecord` | `name`, `symbol` (icône : nom du symbole SF seul, comme `"house.fill"`, ou `"emoji:<un emoji>"` ; `"symbol:<nom>"` est lu aussi ; illisible : `"circle.fill"` ; un symbole inconnu de ce macOS est gardé, et la barre montre le cercle), `theme` (objet `SpaceTheme` : `name`, `colors` = 2 ou 3 × `{red, green, blue}` de 0 à 1, `angle`) ou `null`, `order`, `profileId` |
| `folder` | `FolderRecord` | `name`, `order`, `isCollapsed`, `spaceId`, `parentId` ou `null`, `icon` (`"symbol:<nom du symbole SF>"`, `"emoji:<un emoji>"`, ou `null` pour l'icône par défaut), `textColor` et `backgroundColor` (`"#RRGGBB"` en sRGB, ou `null` pour la couleur du système). Valeur illisible : la valeur par défaut ; un symbole inconnu de ce macOS est gardé, et la barre montre l'icône par défaut |
| `tab` | `TabRecord` : favori, épinglé ou onglet du jour | `place` (`favorite`, `pinned` ou `today`) ; `url` et `title` : pour un favori ou un épinglé, l'adresse et le titre d'origine (`pinnedURL`, `pinnedTitle`), pour un onglet du jour, l'adresse et le titre courants (§ 2.4) ; `http` ou `https` seulement ; `order` ; `profileId` (favori) ou `spaceId` (épinglé, onglet du jour) ; `folderId` (épinglé rangé dans un dossier) ou `null` ; `lastActivity` (onglet du jour : dernière utilisation, § 2.7) |
| `extension` | état d'une extension installée (`installedExtensions`) et, pour une extension installée depuis un fichier, son paquet | § 2.5 |
| `setting` | un réglage de l'app (`UserDefaults`) | `key`, `value` (§ 2.6) |
| `sitePermission` | `SitePermissionRecord` | `origin` (`originKey`, par exemple `https://meet.google.com`, § 2.7), `kind` (`camera` ou `microphone`), `decision` (`allow` ou `deny`, § 2.6), `profileId` |
| `siteZoom` | une entrée de `siteZoomLevels` | `site` (clé du zoom, § 2.7), `level` (de 0,5 à 3) |
| `boost` | `BoostRecord` (fonction reportée, `FEATURES.md`) | `domain`, `css`, `js`, `isEnabled` |

**Restent sur le Mac :**
- historique de navigation (`HistoryRecord`, une ligne par visite : le moteur ignore ses changements dans l'historique de SwiftData) et états de navigation (`interactionState` : pages précédentes et suivantes) ;
- adresse et titre courants d'un favori ou d'un épinglé : seuls l'adresse et le titre d'origine voyagent ;
- regroupements en split view (`splitGroupID`, `splitFraction`) ;
- fenêtres privées, aperçu, mini-fenêtres (jamais enregistrés) ;
- données des sites : cookies, connexions, stockage local ;
- données internes des extensions (`browser.storage`, tenues par WebKit sans API publique pour les lire) : chaque extension a, s'il le faut, sa propre synchro (Bitwarden, uBlock Origin…) ;
- liste et dossier des téléchargements ;
- ce qui dépend du Mac : taille et place des fenêtres, Space affiché, largeur de la barre latérale, raccourci global de la mini-fenêtre, langue de l'app ;
- exceptions du bloqueur intégré (fonction mise de côté).

Un onglet garde son identifiant quand il change de place (un onglet du jour épinglé, un favori rendu aux onglets du jour) : c'est une modification. Fermer un onglet du jour le supprime sur tous les Mac (§ 2.4).

### 2.2 Contenu d'un enregistrement

Un enregistrement porte l'identifiant (`id`, UUID) de son modèle SwiftData. Son contenu est un objet JSON en UTF-8 :

```json
{
  "type": "space",
  "schema": 1,
  "modifiedAt": "2026-10-17T08:30:12.345Z",
  "modifiedBy": "3F2A8C1E-6B0D-4C5E-9F1A-2B7D8E4C6A10",
  "deleted": false,
  "fields": {
    "name": "Perso",
    "symbol": "house.fill",
    "theme": {
      "name": "matcha",
      "colors": [{ "red": 0.55, "green": 0.70, "blue": 0.33 }, { "red": 0.14, "green": 0.21, "blue": 0.10 }],
      "angle": 135
    },
    "order": 0,
    "profileId": "9B1E4D2A-0C3F-4A6B-8E5D-7F2C1A9B3E40"
  }
}
```

- `modifiedAt` est la date de la modification, donnée par l'horloge de la synchro (§ 2.8), en UTC à la milliseconde (§ 2.7).
- `modifiedBy` est l'identifiant de l'appareil (`deviceId`, § 2.7).
- `deleted: true` marque une suppression. `fields` est alors vide.
- Dans `fields`, un champ absent vaut `null`. Les clés sont écrites triées à tous les niveaux, objets imbriqués compris (`JSONEncoder`, `.sortedKeys`).
- `schema` donne la version du format de `fields` pour ce type ; il vaut 1 pour tous les types de la v1. Les champs inconnus d'un `schema` connu sont ignorés, mais gardés et renvoyés tels quels quand ce Mac modifie l'enregistrement.
- **Format plus récent ou type inconnu.** Un contenu dont le `schema` est plus récent que celui du client, ou dont le `type` lui est inconnu, est gardé tel quel dans `SyncState`. Le client ne l'applique pas, et ne le modifie, ne le supprime ni ne le renvoie jamais, même lors d'une opération sur tout le compte (« Remplacer les données synchronisées », curseur expiré, § 5). Si l'objet existe sur ce Mac (`schema` plus récent d'un type connu), il passe en lecture seule dans l'interface : on ne peut plus le renommer, le déplacer ni le supprimer, et Matcha propose de se mettre à jour. Sinon, un renommage écraserait les champs que cette version ne connaît pas.
- **Identifiants tirés d'une clé.** Ce qui n'a pas d'objet propre, ou que deux Mac peuvent créer chacun de leur côté, a un identifiant calculé, le même sur tous les Mac du compte. Il est calculé avec une clé du compte, pour que le serveur ne puisse pas retrouver les sites ou les extensions en essayant des noms :
  - `idKey` = HKDF(accountKey, "matcha.id.v1") (§ 3.2) ;
  - `id` = les 16 premiers octets de HMAC-SHA256(idKey, nom en UTF-8), mis en forme d'UUID version 8 (RFC 9562 § 5.8) : octet 6 = `(x & 0x0F) | 0x80`, octet 8 = `(x & 0x3F) | 0x80`.

  | Type | Nom (UTF-8) |
  | --- | --- |
  | `setting` | `setting:<clé>` |
  | `siteZoom` | `site-zoom:<site>` |
  | `sitePermission` | `site-permission:<profileId en minuscules>\|<origin>\|<kind>` |
  | `extension` installée depuis un fichier | `extension:<identifier>` (§ 2.5) |
  | `extension` Safari d'une app | `extension-app:<bundleIdentifier du .appex, tel quel>` (§ 2.5) |
  | `profile` recréé par la synchro | `default-profile` (§ 2.3, invariants) |
  | `space` recréé par la synchro | `default-space` (§ 2.3, invariants) |

  `site` et `origin` sont normalisés comme au § 2.7. Deux Mac qui règlent la même chose modifient ainsi le même enregistrement, et la règle du § 2.3 les départage. Les autres profils et Spaces, comme les autres types, gardent l'`id` aléatoire de leur modèle.

  **Vérification à la réception :** pour un contenu qui n'est pas une suppression, le client recalcule l'identifiant à partir des champs déchiffrés (`setting`, `siteZoom`, `sitePermission`, `extension`). S'il ne correspond pas, l'enregistrement est ignoré. Une suppression n'a pas de champs : elle n'est pas vérifiée, et s'applique à l'objet de ce Mac dont l'identifiant calculé est celui de la suppression. L'`aad` (§ 3.5) garantit déjà qu'elle a été écrite pour cet identifiant.

### 2.3 Règles

- **Conflits :** enregistrement par enregistrement, la modification la plus récente gagne. On compare `modifiedAt` (à la milliseconde), puis `modifiedBy` en cas d'égalité : le plus grand gagne, les deux UUID étant écrits en minuscules et comparés caractère par caractère. Une suppression est une modification comme une autre : si une modification est plus récente qu'une suppression, elle recrée l'enregistrement.
- **Pas de retour en arrière :** pour chaque enregistrement, le client garde dans `SyncState` le couple (`modifiedAt`, `modifiedBy`) du dernier contenu accepté. Un contenu reçu qui n'est pas plus récent, selon la règle ci-dessus, est ignoré. Un serveur ne peut donc pas rejouer, à un Mac qui l'a déjà reçu, un ancien contenu authentique, comme une autorisation retirée depuis ou une version vulnérable d'une extension. Cette protection vaut seulement tant que `SyncState` est là : un nouveau Mac, ou un Mac qui a perdu ou effacé son état (§ 5), accepte ce que le serveur lui donne.
- **Cascade :** l'appareil qui supprime un profil, un Space ou un dossier envoie aussi la suppression de tout ce qu'il contient (dossiers, onglets, décisions de site du profil).
- **Suppression d'un profil :** à la réception, le Mac efface aussi les données des sites de ce profil (cookies, connexions, stockage local), comme pour une suppression faite sur ce Mac. La confirmation affichée avant de supprimer un profil le dit : « Ce profil, ses cookies et ses connexions seront supprimés sur tous vos Mac. »
- **Ordre d'application** d'un lot reçu : profils, Spaces, dossiers, onglets, extensions, réglages, décisions de site, zooms, Boosts.
- **Parent inconnu :** l'enregistrement reste masqué jusqu'à ce que son parent arrive. Il n'est ni réappliqué à chaque lot, ni pris pour une suppression.
- **Parent supprimé** (sa suppression a été reçue ou faite ici) :
  - le dossier d'un épinglé ou d'un sous-dossier a disparu : l'enfant remonte à la racine des épinglés de son Space (`folderId` ou `parentId` à `null`) ;
  - le Space ou le profil a disparu : l'enfant est supprimé (onglets, dossiers, décisions de site).

  Dans les deux cas, le Mac qui le constate envoie la modification ou la suppression. Si plusieurs Mac l'envoient, la règle des conflits les départage, et le résultat est le même.
- **Cycles de dossiers** (A dans B, B dans A) : le client les détecte à l'application. Dans un cycle, le dossier dont le contenu est le plus récent (même règle que les conflits) remonte à la racine de son Space (`parentId` à `null`), et la modification est envoyée.
- **Space et dossier en désaccord :** si le `folderId` d'un onglet, ou le `parentId` d'un dossier, désigne un dossier d'un autre Space, c'est `spaceId` qui fait foi. L'onglet ou le dossier remonte à la racine des épinglés de son Space, et la modification est envoyée.
- **Invariants :** une session a toujours au moins un profil et un Space (audit R1). Un profil sans Space est permis, comme sur le Mac, où seul le dernier Space de la session ne peut pas être supprimé. Si des suppressions reçues ne laissent aucun profil, le Mac recrée celui du premier lancement (« Personnel ») sous l'identifiant calculé `default-profile`. S'il ne reste aucun Space, il recrée « Perso » sous l'identifiant calculé `default-space`, dans le premier profil (§ 2.7, rangs). Il envoie ensuite ce qu'il a recréé. Ce cas n'arrive qu'avec des suppressions croisées sur deux Mac. Comme les deux Mac recréent les mêmes enregistrements, la règle des conflits n'en garde qu'un, et il n'y a pas de doublon. Si le Space affiché dans une fenêtre est supprimé ailleurs, la fenêtre passe au Space précédent du même profil, sinon au suivant, sinon au premier Space de la session.
- **Rangs :** voir § 2.7. Deux appareils peuvent donner le même `order`. À l'affichage, l'`id` les départage.
- **Valeurs illisibles :** voir § 2.9. Un champ illisible fait soit ignorer l'enregistrement, soit prendre la valeur par défaut au champ.
- **Confiance :** le client se fie au contenu déchiffré (`type`, `deleted`), jamais aux métadonnées envoyées par le serveur. Il n'efface jamais rien sur la foi d'une absence : seule une suppression chiffrée (`deleted: true`) supprime (§ 5).

### 2.4 Onglets du jour

Les onglets du jour de chaque Space voyagent comme une session : on retrouve sur l'autre Mac les onglets ouverts, dans le même ordre.

- **Envois regroupés.** Une page change d'adresse et de titre à chaque navigation. Un onglet du jour n'est envoyé qu'une fois stable depuis 5 secondes, et au plus toutes les 30 secondes s'il change sans cesse ; seuls comptent l'adresse et le titre d'une page chargée (pas les redirections, ni les états de chargement). Ouvrir, fermer, déplacer ou épingler un onglet part au bout de 2 secondes, comme le reste. Un changement de `lastActivity` seul ne part pas : il voyage avec la modification suivante de l'onglet, sinon chaque clic sur un onglet partirait au serveur.
- **À la réception,** un onglet ajouté par un autre Mac prend sa place dans la barre sans être chargé ni sélectionné : il se charge quand on l'ouvre, comme au lancement. Une nouvelle adresse reçue pour un onglet affiché ne change pas la page sous les yeux de l'utilisateur : elle vaut à la prochaine ouverture. Un onglet fermé ailleurs se ferme ici, sauf s'il est affiché : il reste alors à sa place, jusqu'à ce que l'utilisateur le ferme. Un Space reçu s'ouvre sans onglet sélectionné.
- **« Effacer »** (vider les onglets du jour d'un Space) envoie la suppression de chacun de ses onglets.
- Un onglet sans adresse (nouvel onglet vide) voyage avec `url: null`. Seules les adresses `http` et `https` voyagent : un onglet reçu avec une autre adresse (`javascript:`, `file:`, `smb:`…) est ignoré (§ 2.9). Le nom d'utilisateur et le mot de passe d'une adresse (`https://user:password@…`) sont retirés avant l'envoi. Une adresse de ce Mac qui n'est pas web (fichier local, `about:`…) ne part pas : un onglet du jour garde alors sur les autres Mac la dernière adresse web envoyée, et un favori ou un épinglé qui en a une est **local** (§ 5).
- **Fermeture automatique.** Matcha ne ferme aucun onglet du jour d'office (l'archivage est mis de côté, `FEATURES.md`). S'il revient, il faudra revoir cette règle : comme `lastActivity` ne part pas seul, un Mac pourrait fermer, sur la foi d'une date périmée, un onglet qu'un autre Mac utilise.

### 2.5 Extensions

Une extension a un enregistrement, dont l'identifiant est calculé à partir de son `identifier` dans Matcha (§ 2.2). Cet `identifier` est aussi le nom de son dossier d'installation et l'hôte de son adresse (`webkit-extension://<identifier>/`) :

- **Extension installée depuis un fichier** (audit E1) : 32 caractères hexadécimaux en minuscules, tirés au hasard à l'installation et gardés toute sa vie. Une mise à jour signée par le même éditeur (`.crx` dont la signature se vérifie) le garde, comme une mise à jour lancée depuis « Mettre à jour » (ci-dessous). Une extension installée depuis une offre de la synchro reprend celui des autres Mac, s'il est libre sur ce Mac.
- **Extension Safari d'une app** : `safari-` suivi du `bundleIdentifier` du `.appex`, mis en minuscules et sans accents. Chaque suite de caractères hors `a-z` et `0-9` devient un `-`, sans `-` au début ni à la fin. Le tout est tronqué à 63 caractères, la longueur maximale d'un identifiant. Il est le même sur tous les Mac. Deux `bundleIdentifier` différents peuvent donner le même `identifier` (`com.a.b` et `com.a-b`). C'est pourquoi l'enregistrement d'une extension Safari tire son identifiant du `bundleIdentifier` exact (`extension-app:…`, § 2.2), et non de l'`identifier`. Sur un Mac, une seconde extension Safari dont l'`identifier` est déjà pris n'est pas chargée : Réglages > Extensions dit « Une autre extension porte déjà cet identifiant ».

Ni l'identifiant gecko ni `manifest.key` ne servent : n'importe qui peut les recopier dans un faux paquet. Le même fichier installé à part sur deux Mac donne donc deux extensions, avec deux identifiants. Chaque Mac voit alors l'autre copie sous « Depuis vos autres Mac », avec la mention « Une extension du même nom est installée sur ce Mac », et peut l'ignorer.

L'objet ci-dessous est le `fields` d'une extension (`schema` 1) :

```json
{
  "identifier": "3f9c0d6e1b2a4c5d8e7f6a5b4c3d2e1f",
  "name": "Dark Reader",
  "version": "4.9.99",
  "source": {
    "kind": "package",
    "blobId": "5A1C9E3B-7D2F-4B8A-9C6E-1F0D3B7A2E58",
    "blobKey": "base 64 de 32 octets",
    "sha256": "empreinte SHA-256 du paquet, 64 caractères hexadécimaux minuscules",
    "size": 845312,
    "fileName": "darkreader.xpi"
  },
  "isEnabled": true,
  "allowsPrivateWindows": false,
  "showsButton": true,
  "grantedPermissions": { "tabs": null, "storage": null },
  "deniedPermissions": {},
  "grantedMatchPatterns": { "*://*/*": null },
  "deniedMatchPatterns": {}
}
```

- **`version`** est celle du manifeste (`version`, pas `version_name`).
- **`source`** dit d'où vient l'extension :
  - `"kind": "appExtension"` : extension Safari d'une app, avec `bundleIdentifier` (celui du `.appex`), `appBundleIdentifier` et `appName`. Rien n'est envoyé d'autre : chaque Mac la charge depuis son app.
  - `"kind": "package"` : installée depuis un fichier ou un dossier. Le paquet (archive zip du dossier installé, 20 Mio au plus) est envoyé chiffré à part (§ 4.7) ; `blobKey`, tirée au hasard pour chaque version du paquet, ne voyage que dans l'enregistrement chiffré.
- **Permissions** : nom de la permission ou motif d'adresses → date d'expiration (§ 2.7), ou `null` sans expiration. Les droits reçus peuvent seulement réduire ceux de ce Mac : un refus ou un retrait reçu (permission refusée, site retiré) s'applique, mais un droit accordé ailleurs ne s'ajoute pas ici. L'extension le redemande sur ce Mac quand elle en a besoin.
- **`allowsPrivateWindows`** : la synchro peut retirer l'accès aux fenêtres privées, jamais le donner. Il ne s'accorde que sur ce Mac.
- **Ce qui n'est pas appliqué reste dans l'enregistrement** (règle générale au § 5). Quand ce Mac envoie l'enregistrement d'une extension, il y laisse tout ce qu'il a reçu sans l'appliquer : `version` et `source` d'une mise à jour pas encore installée, droits, accès aux fenêtres privées. Seul un changement fait sur ce Mac les remplace. Sinon, la première modification faite ici renverrait l'ancienne version et retirerait ces droits. Par exemple, B reçoit la v2 sans l'installer, puis désactive l'extension. Sans cette règle, B renverrait la v1, et A se verrait proposer un retour à la v1, dont il a déjà supprimé le paquet.
- **Une extension reçue qui n'est pas installée sur ce Mac** n'est ni installée ni chargée d'office : Réglages > Extensions la montre sous « Depuis vos autres Mac », et l'utilisateur l'ajoute après l'écran des permissions (code et accès venus d'ailleurs : un compte ou un Mac compromis ne doit pas pouvoir en installer). Un paquet est alors téléchargé, déchiffré, vérifié (`sha256`, taille) et installé comme un fichier ; s'il manque (§ 4.7), l'extension est montrée « à réinstaller » ; une extension Safari est chargée depuis son app si elle est là, sinon Matcha propose son app dans l'App Store (adresse trouvée par l'API publique de recherche d'iTunes, `https://itunes.apple.com/lookup?bundleId=<appBundleIdentifier>`). « Ignorer » la retire de la liste sur ce Mac seulement (pour cet identifiant : elle n'est plus proposée).
- **Une extension déjà installée ici** suit les réglages reçus (active, bouton), avec les limites ci-dessus pour les droits et les fenêtres privées. Un nouveau paquet (`sha256` différent) ne s'installe **jamais** seul, même s'il ne demande aucun droit de plus : un Mac compromis pourrait sinon remplacer le code d'une extension sur tous les Mac, en gardant ses droits et ses données. Réglages > Extensions montre « Mise à jour depuis vos autres Mac », et l'utilisateur l'installe d'un clic, après un écran qui montre le passage de la version a à la version b, les droits ajoutés et, si la version baisse, un avertissement. Un paquet `.crx` signé par un autre éditeur est un remplacement (audit E1). « Ignorer cette version » garde la version installée ici : l'enregistrement continue de citer la nouvelle (§ 5), et ce Mac ne la propose plus (il retient son `sha256`). Une version suivante est proposée de nouveau.
- **Mise à jour faite à la main sur un Mac.** Un fichier signé par le même éditeur garde l'`identifier`. Un fichier qu'on ne peut pas vérifier (`.xpi`, `.zip`, dossier, `.crx` non signé) le garde aussi s'il est choisi depuis le bouton « Mettre à jour » de l'extension installée. Un écran montre alors le passage de la version a à la version b, les droits ajoutés et la mention « Ce fichier ne peut pas être vérifié ». Dans les deux cas, c'est une modification de l'enregistrement : les données de l'extension sont gardées, et les autres Mac la reçoivent sous « Mise à jour depuis vos autres Mac », jamais installée d'office. C'est sûr : l'utilisateur de ce Mac choisit lui-même l'extension à mettre à jour, et le code venu d'un autre Mac demande toujours confirmation. Un fichier ouvert ou déposé autrement est une nouvelle installation, avec un nouvel `identifier`, même si le nom est le même. Un `.crx` signé par un autre éditeur reste un remplacement, même depuis « Mettre à jour » (audit E1) : nouvel `identifier`, donc nouvel enregistrement, et l'ancien est supprimé. Les autres Mac désinstallent alors l'ancienne extension, avec ses données, et reçoivent la nouvelle sous « Depuis vos autres Mac ».
- **Désinstaller** sur un Mac supprime l'enregistrement : l'extension est désinstallée partout (ses données internes avec), et le paquet est supprimé du stockage (§ 4.7).

### 2.6 Réglages et sites

- **`setting`** : un enregistrement par réglage (identifiant tiré de la clé, § 2.2), `value` étant la valeur telle que l'app la garde (texte, booléen ou nombre). Réglages synchronisés :

  | Clé | Valeurs |
  | --- | --- |
  | `searchEngine` | `google`, `duckDuckGo`, `bing` |
  | `appearance` | `system`, `light`, `dark` |
  | `confirmBeforeQuitting`, `showsHomeButton`, `showsNavigationButtons`, `showsSearchSuggestions`, `opensPinnedLinksInPeek` | booléen |
  | `homePage` | texte (adresse saisie, ou vide) ; une adresse avec un schéma autre que `http` ou `https` est ignorée |
  | `newTabPage` | `blank`, `homePage` |
  | `addressBarPosition` | `sidebar`, `top` |
  | `tabSuspensionDelay` | nombre de minutes : `15`, `30`, `60`, `120`, ou `0` (jamais) |
  | `externalLinkDestination` | `miniWindow`, `space` |

  Une clé hors de cette liste, ou une valeur inconnue, est ignorée à la réception. Un nombre doit être un entier exact : `1e19` ou `15.5` sont des valeurs inconnues. Ajouter un réglage à la liste ne demande pas de nouveau `schema`.
- **`sitePermission`** : la décision d'un profil pour une origine et une permission. Revenir à « Demander » supprime l'enregistrement. **Seuls les refus s'appliquent à la réception.** Un `deny` reçu, ou la suppression d'un `allow`, s'applique sur ce Mac. Un `allow` reçu n'ouvre rien : ce Mac continue de demander, et la réponse donnée ici repart comme une décision de ce Mac. Sinon, un Mac compromis pourrait ouvrir la caméra ou le micro à un site sur tous les Mac. Un `allow` gardé sur ce Mac et remplacé par un `deny` reçu devient un refus. Un `allow` reçu et non appliqué n'est pas une suppression locale : ce Mac n'envoie pas sa suppression (§ 5), et l'autre Mac garde son autorisation.
- **`siteZoom`** : le zoom retenu pour un site (commun à tous les profils, comme sur le Mac). Revenir à 100 % supprime l'enregistrement.

### 2.7 Formats communs

Ces formats sont figés en v1. Deux versions de Matcha qui les écriraient différemment ne retrouveraient pas les mêmes enregistrements.

- **Dates** (`modifiedAt`, `lastActivity`, expiration d'un droit d'extension) : ISO 8601 en UTC, toujours avec trois chiffres de millisecondes et `Z`, par exemple `2026-10-17T08:30:12.345Z`.
- **UUID** : écrits en majuscules dans les contenus (forme de `UUID.uuidString`), lus sans tenir compte de la casse, et mis en minuscules dans les noms du § 2.2 et dans l'`aad` (§ 3.5).
- **Données binaires et empreintes** : `blobKey` en base 64 standard avec complément ; `sha256` en 64 caractères hexadécimaux minuscules (empreinte du paquet en clair, avant complément) ; `size` en octets, entier.
- **Identifiant de l'appareil** : le même UUID partout. C'est le `modifiedBy` des contenus, le `device.id` des requêtes et le `deviceId` de la Session (§ 4.3). Il est tiré à la première activation de la synchro et gardé dans `SyncState` et dans le trousseau (§ 3.8, et § 5 pour un Mac cloné).
- **`order`** : un nombre JSON, décimal permis (double IEEE 754), toujours fini. Chacun des groupes suivants forme une seule suite :
  - les profils ;
  - les Spaces, tous profils confondus ;
  - les favoris d'un profil ;
  - les épinglés et les dossiers d'un même parent (racine des épinglés d'un Space, ou un dossier) ;
  - les onglets du jour d'un Space.

  Pour un déplacement, le client donne à l'élément déplacé une valeur entre celles de ses deux voisins, pour n'envoyer qu'un seul enregistrement. Si l'écart devient trop petit (moins de 10⁻⁶), il renumérote tout le groupe (0, 1, 2…) et envoie chacun de ses éléments.
- **Origine (`origin`, décisions de site)** : la clé de `SiteOrigin`, la même pour les décisions de site du Mac et pour la synchro. Elle s'écrit `<schéma>://<hôte>[:<port>]` :
  - schéma `http` ou `https` seulement ;
  - schéma et hôte en minuscules, hôte en ASCII (punycode `xn--…`), sans point final, une adresse IPv6 entre crochets ;
  - le port est retiré s'il est celui du schéma (80, 443) ;
  - ni chemin, ni `/` final ;
  - `www.` est gardé : `www.example.com` et `example.com` sont deux origines.

  Par exemple : `https://meet.google.com`, `http://localhost:8080`, `http://[::1]:3000`.
- **Site (`site`, zooms)** : la clé de `SiteZoomKey`, la même pour les zooms du Mac et pour la synchro. Pour une page web, c'est le domaine enregistrable, approché par `SiteDomain` :
  - les deux derniers libellés de l'hôte, ou les trois derniers sous un suffixe connu à deux libellés (`bbc.co.uk`) ;
  - en minuscules, en ASCII, sans point final ;
  - ni schéma, ni port, ni `www.` ;
  - une adresse IP, `localhost` ou un nom sans point est pris en entier.

  Tous les fichiers locaux partagent la clé `file://`. La liste des suffixes peut changer d'une version à l'autre : le client prend `site` tel qu'il est reçu et ne le recalcule jamais à partir d'une adresse. Au pire, deux versions qui ne découpent pas un hôte de la même façon créent deux zooms pour un même site.
- **`lastActivity`** : un changement de `lastActivity` seul ne déclenche pas d'envoi (§ 2.4).

### 2.8 Horloge

- **Horloge hybride :** `modifiedAt` est la plus grande valeur entre l'heure du Mac et la plus grande date connue + 1 ms. Les dates connues sont celles écrites par ce Mac et celles reçues, sauf exception ci-dessous. Cette plus grande date est gardée dans `SyncState` et survit à un redémarrage. Un appareil qui a reçu une modification écrit toujours après elle, même si son horloge retarde.
- **Après la version remplacée :** une modification locale d'un enregistrement est toujours datée au moins 1 ms après le contenu qu'elle remplace, même si l'horloge donne une date plus ancienne. Sans cela, elle perdrait le conflit contre ce contenu et serait effacée.
- **Date trop loin dans le futur :** une date reçue à plus de 24 h devant l'heure de ce Mac ne fait pas avancer son horloge. Elle compte pour son enregistrement (conflits, § 2.3), mais ne se propage pas aux autres. Sans ce garde-fou, un Mac réglé en 2030 gagnerait tous les conflits, et sa date se propagerait à tous les Mac. Le client le journalise, et Réglages > Synchronisation affiche « L'horloge d'un de vos Mac semble en avance. »
- **Mac déréglé :** à chaque réponse du serveur, le client compare son heure à l'en-tête `Date` (RFC 9110). Au-delà de 10 minutes d'écart, les réglages affichent « L'horloge de ce Mac est décalée ». Au-delà de 24 h d'avance, le client n'envoie plus rien jusqu'à ce que l'horloge soit corrigée. Il continue de lire. L'en-tête `Date` n'est pas authentifié : il ne sert qu'à prévenir, jamais à dater.

### 2.9 Valeurs illisibles

Un champ absent, du mauvais type ou hors des valeurs connues est **illisible**. Selon le champ, soit l'enregistrement entier est ignoré, soit le champ prend sa valeur par défaut.

- **Enregistrement ignoré.** Il n'est ni appliqué, ni écrasé, ni supprimé, et reste dans `SyncState` tel qu'il a été reçu. Les opérations sur tout le compte (§ 5) n'y touchent pas non plus.
- **Valeur par défaut.** Le reste de l'enregistrement s'applique. Le champ ne repart avec sa valeur par défaut que si ce Mac modifie l'enregistrement.

| Type | Champs qui font ignorer l'enregistrement | Champs qui prennent leur valeur par défaut |
| --- | --- | --- |
| tous | contenu indéchiffrable (§ 3.5), `type`, `schema`, `modifiedAt`, `modifiedBy`, `deleted`, identifiant calculé qui ne correspond pas (§ 2.2 ; jamais pour une suppression) | — |
| `profile` | — | `name` (`""`), `order` (0) |
| `space` | `profileId` | `name` (`""`), `symbol` (`circle.fill`), `theme` (`null` : thème par défaut), `order` (0) |
| `folder` | `spaceId` | `name` (`""`), `order` (0), `isCollapsed` (`false`), `parentId` (`null`), `icon`, `textColor`, `backgroundColor` (`null`) |
| `tab` | `place` ; le parent que demande la place (`profileId` pour un favori, `spaceId` sinon) ; `url` autre que `null`, `http` ou `https` | `title` (`null`), `order` (0), `folderId` (`null`), `lastActivity` (`null`) |
| `extension` | `identifier` ; `source.kind` ; pour un paquet, `blobId`, `blobKey` (32 octets), `sha256`, `size` ; pour une extension Safari, `bundleIdentifier` et `appBundleIdentifier` | `name`, `version`, `fileName`, `appName` (`""`) ; `isEnabled` et `allowsPrivateWindows` (`false`) ; `showsButton` (`true`) ; listes de droits (`{}`), date d'expiration illisible : le droit est ignoré |
| `setting` | `key`, `value` (§ 2.6) | — |
| `sitePermission` | `origin` (pas sous la forme du § 2.7), `kind`, `decision`, `profileId` | — |
| `siteZoom` | `site`, `level` (nombre de 0,5 à 3) | — |

Un `symbol` ou une `icon` d'un symbole SF inconnu de ce macOS n'est pas illisible : la valeur est gardée, et la barre latérale montre l'icône par défaut (§ 2.1).

---

## 3. Chiffrement (serveurs privé et public)

### 3.1 Primitives

| Usage | Algorithme | API sur macOS |
| --- | --- | --- |
| Mot de passe → clé maître | PBKDF2-HMAC-SHA256, de 600 000 à 10 000 000 itérations, sel aléatoire de 16 octets, sortie de 32 octets | `CCKeyDerivationPBKDF` (CommonCrypto) |
| Identifiants tirés d'une clé | HMAC-SHA256 | `HMAC<SHA256>` (CryptoKit) |
| Sous-clés | HKDF-SHA256, sel vide, sortie de 32 octets | `HKDF<SHA256>` (CryptoKit) |
| Chiffrement | AES-256-GCM, nonce aléatoire de 12 octets, étiquette de 16 octets | `AES.GCM` (CryptoKit) |
| Aléa | générateur du système | `SymmetricKey(size:)`, `SecRandomCopyBytes` |

Notations : `HKDF(k, "info")` donne une sous-clé de 32 octets. `Seal(k, données, aad)` donne `nonce ‖ chiffré ‖ étiquette`.

Le mot de passe maître est normalisé en Unicode NFKD, puis encodé en UTF-8. Il fait au moins 12 caractères, comptés en graphèmes (`String.count` de Swift) avant la normalisation : « é » compte pour un, qu'il soit composé ou non.

Paramètres `kdf` acceptés par le client comme par le serveur : `algorithm` = `pbkdf2-sha256` (tout autre nom est refusé), `iterations` de 600 000 à 10 000 000, `salt` de 16 octets exactement. Au-delà du maximum, un serveur malveillant pourrait figer le client dans un calcul sans fin.

### 3.2 Clés

| Clé | Obtenue par | Reçue par le serveur | Rôle |
| --- | --- | --- | --- |
| `masterKey` | PBKDF2(mot de passe, `salt`, `iterations`) | non | dérive `authKey` et `wrapKey` ; jamais enregistrée |
| `authKey` | HKDF(masterKey, "matcha.auth.v1") | oui (il n'en garde qu'un hachage) | prouve la connaissance du mot de passe |
| `wrapKey` | HKDF(masterKey, "matcha.wrap.v1") | non | enveloppe la clé de compte |
| `accountKey` | 32 octets aléatoires, tirés à la création du compte | non | clé de toutes les données ; ne change pas avec le mot de passe |
| `dataKey` | HKDF(accountKey, "matcha.data.v1") | non | chiffre les enregistrements |
| `idKey` | HKDF(accountKey, "matcha.id.v1") | non | calcule les identifiants tirés d'une clé (§ 2.2) |
| `recoveryKey` | 20 octets aléatoires, tirés à la création du compte | non | clé de secours, gardée par l'utilisateur |
| `recoveryAuthKey` | HKDF(recoveryKey, "matcha.recovery-auth.v1") | oui (hachage) | prouve la connaissance de la clé de secours |
| `recoveryWrapKey` | HKDF(recoveryKey, "matcha.recovery-wrap.v1") | non | enveloppe aussi la clé de compte |

Le serveur stocke deux enveloppes de la clé de compte :

- `wrappedAccountKey` = Seal(wrapKey, accountKey, "matcha.account-key.v1")
- `recoveryWrappedAccountKey` = Seal(recoveryWrapKey, accountKey, "matcha.account-key.v1")

La clé de secours s'affiche en base 32 de Crockford : 32 caractères en 8 groupes de 4 (`7KQ2-M9XD-…`). À la saisie, la casse, les espaces et les tirets sont ignorés ; `O` vaut `0`, `I` et `L` valent `1`. `U` n'est pas dans l'alphabet : comme tout autre caractère hors de l'alphabet, il rend la clé invalide, et l'écran dit « Caractère non valide ».

Côté serveur, `authKey` et `recoveryAuthKey` sont hachées avec Argon2id (paramètres au § 4.9). Le serveur ne garde jamais la valeur reçue.

### 3.3 Création du compte

1. `GET /v1/server` : vérifier la version de l'API et le mode d'inscription.
2. Tirer `salt`, `accountKey` et `recoveryKey`, puis calculer les clés du § 3.2.
3. `POST /v1/accounts` avec `kdf`, `authKey`, `recoveryAuthKey` et les deux enveloppes.
4. Afficher le kit de secours : adresse du serveur, identifiant, clé de secours. L'utilisateur doit l'enregistrer en PDF ou l'imprimer avant de continuer.
5. Garder `accountKey` dans le trousseau. Oublier le mot de passe, `masterKey` et `recoveryKey`.

### 3.4 Connexion d'un autre Mac

1. `POST /v1/auth/prelogin` renvoie les paramètres `kdf` du compte. Le client refuse des paramètres hors des bornes du § 3.1 : un serveur malveillant ne peut ni affaiblir le calcul pour deviner plus vite le mot de passe, ni l'alourdir pour figer le client.
2. Calculer `masterKey`, `authKey` et `wrapKey`.
3. `POST /v1/auth/login` renvoie les jetons et `wrappedAccountKey`.
4. Ouvrir `wrappedAccountKey` avec `wrapKey`. Le serveur refuse déjà un mauvais mot de passe à l'étape 3 ; un échec ici veut donc dire que l'enveloppe a été altérée.

### 3.5 Chiffrement d'un enregistrement

1. Encoder le contenu JSON (§ 2.2) en UTF-8, puis le compléter jusqu'à un multiple de 256 octets : un octet `0x80`, puis des `0x00` (ISO/IEC 7816-4). Le complément cache la taille exacte.
2. `aad` = `matcha.record.v1|<id en minuscules>|<keyVersion>`. Un contenu recopié sous un autre identifiant ne s'ouvre pas.
3. `payload` = `0x01` (version du format) ‖ Seal(dataKey, contenu complété, aad), en base 64.

À la lecture : vérifier l'octet de version, ouvrir, retirer le complément, décoder. Si une étape échoue, l'enregistrement est ignoré et jamais écrasé. L'erreur est journalisée et la synchro affiche « Données illisibles ».

`keyVersion` vaut 1. Il est réservé à une future rotation de la clé de compte (nouvelle `accountKey`, tous les enregistrements rechiffrés). Une rotation change aussi `idKey`, donc les identifiants tirés d'une clé : ces enregistrements seront recréés sous leur nouvel identifiant, et les anciens supprimés.

**Paquet d'extension** (§ 2.5) : sa propre clé, `blobKey` (32 octets aléatoires par version du paquet), voyage dans l'enregistrement de l'extension, donc chiffrée par `dataKey`. Le paquet est complété jusqu'à un multiple de 64 Kio (même complément qu'au point 1), puis `0x01` ‖ Seal(blobKey, paquet complété, `matcha.blob.v1|<blobId en minuscules>`), envoyé tel quel en binaire. À la lecture, le client vérifie aussi l'empreinte `sha256` et la taille annoncées par l'enregistrement.

### 3.6 Changer le mot de passe maître

Un nouveau `salt` donne de nouvelles `masterKey`, `authKey` et `wrapKey`. Seule `accountKey` est réenveloppée : les données ne sont pas rechiffrées (`POST /v1/account/password`). Les autres appareils sont déconnectés et redemandent le mot de passe.

### 3.7 Mot de passe oublié

1. L'utilisateur saisit sa clé de secours. `POST /v1/auth/recover/start` avec `recoveryAuthKey` renvoie `recoveryWrappedAccountKey` et un jeton de récupération valable 10 minutes.
2. Le client ouvre l'enveloppe avec `recoveryWrapKey` et retrouve `accountKey`. L'utilisateur choisit un nouveau mot de passe, et le client prépare `kdf`, `authKey` et `wrappedAccountKey` comme au § 3.3.
3. `POST /v1/auth/recover/finish`. Les autres appareils sont déconnectés. Le client propose de créer une nouvelle clé de secours.

Si le mot de passe et la clé de secours sont tous les deux perdus, les données sont perdues. Il ne reste qu'à supprimer le compte et à repartir des données d'un Mac.

### 3.8 Sur le Mac

- **Trousseau** (`kSecClassGenericPassword`, service `com.matchabrowser.matcha.sync`, trousseau de session de ce Mac, jamais synchronisé par iCloud ; dans le bac à sable, ses éléments ne sont lisibles que par Matcha) : `accountKey`, jetons, identifiant de l'appareil et marque de la machine (§ 5). La clé de compte et les jetons sont rangés par serveur et par compte : `kSecAttrAccount` = `<adresse normalisée du serveur (§ 4.1)>|<accountId en minuscules>|<élément>`. Changer de serveur ou de compte ne mélange donc rien. L'identifiant de l'appareil et la marque de la machine valent pour tout le Mac (`kSecAttrAccount` = `device`). Une erreur du trousseau (trousseau verrouillé, accès refusé) est une erreur, et non une absence : la synchro attend, sans se croire déconnectée. Le trousseau protégé par l'appareil (`kSecUseDataProtectionKeychain`, `kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly`) demande un identifiant d'app signé par un profil de provisionnement : à reprendre si l'app en a un.
- **Jamais enregistrés :** mot de passe maître, `masterKey`, `recoveryKey`.
- **Base SwiftData :** en clair, comme aujourd'hui, protégée par la session macOS et FileVault.

### 3.9 Ce que voit le serveur

**Il voit :** l'identifiant (adresse e-mail), les paramètres `kdf`, les hachages d'`authKey` et de `recoveryAuthKey`, les enveloppes de la clé de compte, le nom et les dates des appareils. Pour chaque enregistrement, il voit aussi l'identifiant, la version, l'indicateur de suppression, la taille arrondie à 256 octets et la date d'écriture. Pour chaque paquet d'extension (§ 4.7), il voit son identifiant, sa taille arrondie à 64 Kio et ses dates.

**Il ne voit pas :** le type des enregistrements, ni aucun nom, adresse, onglet, réglage, site, extension, thème ou Boost, ni aucun contenu. Les identifiants tirés d'une clé ne lui apprennent rien : sans `idKey`, il ne peut pas les recalculer à partir d'un nom de site ou de réglage (§ 2.2).

**Limites connues de la v1 :**
- Le serveur reçoit `authKey`. S'il est malveillant, il peut tester des mots de passe hors ligne, au prix d'un calcul PBKDF2 par essai. La longueur minimale et la jauge de solidité servent à ça.
- Un serveur malveillant peut cacher des enregistrements ou des suppressions sans être détecté. Il ne peut en revanche ni lire, ni fabriquer, ni déplacer un contenu, ni rien effacer en « oubliant » des enregistrements (§ 5). Il ne peut pas non plus rejouer une version plus ancienne que celle qu'un Mac a déjà acceptée (§ 2.3). En revanche, il le peut face à un Mac sans état : un nouveau Mac, ou un Mac reconnecté après avoir effacé son `SyncState`.
- Le paquet d'une extension part juste avant l'enregistrement qui le cite (§ 4.7) : les horaires suffisent au serveur pour deviner quel enregistrement renvoie à quel paquet, donc qu'un enregistrement est une extension installée depuis un fichier, et la taille approximative de son paquet.
- L'inscription révèle si une adresse e-mail a déjà un compte (`email_taken`), même si `prelogin` ne le révèle pas (§ 4.3). Un serveur ouvert à tous devra limiter le débit des inscriptions, ou vérifier l'adresse avant de répondre (§ 7).

### 3.10 Vecteurs de test

Calculés par une implémentation indépendante (Python `cryptography`) ; un client ou un serveur doit les retrouver (`SyncCryptoTests`). Entrées : mot de passe `Café crème, 2026 !` (NFKD : `43616665cc8120637265cc806d652c20323032362021`), `salt` = octets 0 à 15, 600 000 itérations ; `accountKey` = octets 32 à 63 ; `recoveryKey` = octets 64 à 83 ; nonces = octets 100 à 111 (enveloppe), 112 à 123 (enveloppe de secours), 124 à 135 (enregistrement), 136 à 147 (paquet).

| Valeur | Résultat |
| --- | --- |
| `masterKey` | `1b17deb72bf81bc32f13fa4e0ce6e2458325b151098172bf5f60579d1725872a` |
| `authKey` | `5181174923363be5cfc1ddd47761224fe7653b627bb8a16b9b56e44cbf3bb477` |
| `wrapKey` | `ef4e8ca94c79a8831d0be6956954257ea820aae32daad637623c33d573978bbd` |
| `dataKey` | `681021aae32d4b8896879d3b68426f203ffe80a3505f4f88688e393b71c8da7c` |
| `idKey` | `1566d83727cc0f51bd6130bcfbeb63b5edeed74d0aa752c98507d8610a539dac` |
| `recoveryAuthKey` | `e09957c068a62d3d182a3eb20567f6081ae9d5ad89034125ffc2376aad9b2ee0` |
| `recoveryWrapKey` | `c99ca23f11c93fad7ae8bfc65d45937f1d3ba0bbb9f47770d68dccc3c41ee275` |
| `wrappedAccountKey` (base 64) | `ZGVmZ2hpamtsbW5vtSkcc3+EmMJgIoX82mvpDSrkO645VA8Tv2uuthfFNFVN2Rq2ZgR73kssLbiYmJgk` |
| `recoveryWrappedAccountKey` (base 64) | `cHFyc3R1dnd4eXp7s5N8UZFUOtzOSzjCwNalJVB4uWQ2w4/Y3OMaYjcFGDaG8ywQwJ9tnr7Xz4peNUdl` |
| Clé de secours affichée | `810M-4GT4-8N34-EJ29-995M-RKAE-9X85-2MJK` |

**Identifiants tirés d'une clé** (§ 2.2), avec l'`idKey` ci-dessus :

| Nom | Identifiant |
| --- | --- |
| `setting:searchEngine` | `29C6E709-25C1-837D-A81F-9938153247BB` |
| `site-zoom:google.com` | `D841E19B-C45E-8758-A7F8-EE09968642E7` |
| `site-permission:9b1e4d2a-0c3f-4a6b-8e5d-7f2c1a9b3e40\|https://meet.google.com\|camera` | `3574A498-BBBE-89DD-97E5-E8B259C7B0BF` |
| `extension:0123456789abcdef0123456789abcdef` | `B146F347-53E0-8D14-B690-A7E7477085F7` |
| `extension-app:com.example.DarkReader.Extension` | `B57FF5FB-0A29-8DBD-A38F-18FFB8646F9E` |
| `default-profile` | `F0C1AD76-39FD-8912-B97C-7D330800F8F3` |
| `default-space` | `8467CEA7-A22F-842E-AF9E-B52B799680AE` |

**Enregistrement chiffré** : identifiant `7C4B2E9A-1D3F-4B6E-8A0C-5E2D9F1B3A76`, `keyVersion` 1, `dataKey` ci-dessus, nonce = octets 124 à 135. Contenu (UTF-8, 177 octets, complété jusqu'à 256) :

```json
{"deleted":false,"fields":{"name":"Personnel","order":0},"modifiedAt":"2026-10-17T08:30:12.345Z","modifiedBy":"3F2A8C1E-6B0D-4C5E-9F1A-2B7D8E4C6A10","schema":1,"type":"profile"}
```

`payload` (285 octets : 1 + 12 + 256 + 16) :

```
AXx9fn+AgYKDhIWGhy4GlMRzc+YJeTnjZErem3VDPTwk4WRTSxyMJDg2dKkUu4uAoKOZt3k+uh7IcGj4HGY2C4227yL/t64q
qc7gcBc4I8Nl/V1fUWESR9zeh72w6IDlRn8EZRMvtIEJCzt7F59kiu0eaWbgjf4rFD76riMiHHt/87v3+llWYu1G0aOXCi+/
smVS94aTfQOi+AmaSeEP+nb3zuE6/IfBEG6SrlO4rHaAljgF50eyLRxUKdDRS6bER/9J48TEGAtO5IZxFYAUESyey9ikCWO3
hnFvY/0XIyfSmOZ4aoeZmy3jPGSW6LeHhrsoAc/XdAXlRa8H6KRE2DBHSTqF0rVRlvVOweBtG/adxKH7kcFuGRDgEA1p
```

(base 64 sur une seule ligne dans l'API ; coupée ici pour la lecture.)

**Paquet d'extension** : identifiant `5A1C9E3B-7D2F-4B8A-9C6E-1F0D3B7A2E58`, clé (`blobKey`) = octets 150 à 181, nonce = octets 136 à 147. Paquet = `50 4B 03 04` suivi quatre fois des octets 0 à 255 (1 028 octets ; SHA-256 `55cf14b0da2830b4a0c792188f89ecb6d7331b8260a9af1b6fc5a65abf78adcc`), complété jusqu'à 65 536 octets. Paquet chiffré : 65 565 octets (1 + 12 + 65 536 + 16), SHA-256 `21c61a10b905ebc08287797aac0bf30e30134f9f21af66e71ccb185e492f4a85`.

---

## 4. API HTTP v1

### 4.1 Conventions

- **Adresse :** `https://<hôte>[:<port>][/<chemin>]/v1/…`. Le serveur de référence répond sous `/api` (`https://sync.example.com/api/v1/…`). HTTPS est obligatoire ; le client n'accepte `http` que pour `localhost`, `127.0.0.1` et `::1`. L'adresse saisie dans les réglages est normalisée :
  - `https://` est ajouté si l'adresse n'a pas de schéma ;
  - schéma et hôte en minuscules, port par défaut retiré ;
  - le chemin est gardé, sans `/` final, sans `?` ni `#` ;
  - l'utilisateur ne tape pas `/v1` : le client l'ajoute.

  L'adresse n'est retenue qu'après une réponse valide à `GET /v1/server` (`api` = 1). C'est sous cette forme normalisée qu'elle figure dans le trousseau (§ 3.8) et dans le kit de secours.
- **Certificats :** le client se fie seulement aux certificats que macOS reconnaît (vérification par défaut d'`URLSession`). Il n'a ni bouton « Continuer quand même », ni exception par serveur, ni épinglage de certificat. Un serveur installé chez soi a trois solutions :
  - un certificat public, par exemple Let's Encrypt avec un défi DNS, qui marche même sur le réseau local sans ouvrir de port ;
  - un réseau privé qui fournit les certificats (Tailscale) ;
  - sa propre autorité de certification. On ajoute son certificat dans Trousseaux d'accès (trousseau « session », ou « Système » pour tous les comptes du Mac), puis Lire les informations > Se fier > « Lors de l'utilisation de ce certificat » : « Toujours approuver ». Le certificat du serveur doit répondre aux exigences de macOS : l'hôte dans `subjectAltName`, l'usage `serverAuth`, une clé RSA d'au moins 2 048 bits ou ECDSA, une signature en SHA-2 (un certificat signé en SHA-1 est refusé), et 825 jours de validité au plus.

  Un certificat auto-signé seul, sans autorité installée, est refusé. Le message dit pourquoi, et renvoie à l'aide.
- **Format :** JSON en UTF-8 (`Content-Type: application/json`). Données binaires en base 64 standard, avec complément. Dates en ISO 8601, UTC, à la milliseconde. Les UUID se comparent sans tenir compte de la casse ; l'`aad` (§ 3.5) utilise leur forme en minuscules.
- **Identifiant :** l'adresse e-mail, sans espaces autour et en minuscules (normalisée par le client comme par le serveur). Un serveur privé n'est pas obligé de la vérifier.
- **Authentification :** `Authorization: Bearer <accessToken>` sur toutes les routes, sauf `GET /v1/server`, `POST /v1/accounts`, `POST /v1/auth/prelogin`, `POST /v1/auth/login`, `POST /v1/auth/refresh`, `POST /v1/auth/recover/start` et `POST /v1/auth/recover/finish`. `POST /v1/auth/logout` est authentifié.
- **Client :** en-tête `Matcha-Client: <version> (<build>)` sur chaque requête. Un serveur peut refuser une version trop ancienne (`unsupported_client`).
- **Jetons :** opaques pour le client. Le jeton d'accès dure 15 minutes. Le jeton de renouvellement dure 60 jours glissants et ne sert qu'une fois. S'il est réutilisé, le serveur révoque l'appareil (vol probable). Le client ne fait donc **qu'un renouvellement à la fois** : le cycle de synchro, le flux d'événements et le téléchargement d'un paquet attendent le même renouvellement en cours, au lieu d'en lancer un chacun. Le nouveau jeton de renouvellement est écrit dans le trousseau avant d'être utilisé.
- **Opérations sensibles :** changer le mot de passe ou la clé de secours, supprimer le compte. Elles exigent `authKey` en plus du jeton, ce qui revient à redemander le mot de passe.
- **Erreurs :** `application/problem+json` (RFC 9457) avec un `code` stable (§ 4.8). Le client affiche son propre message, traduit, d'après le `code`.
  ```json
  { "status": 401, "code": "invalid_credentials", "title": "Invalid credentials" }
  ```
- **Limites par défaut**, annoncées par `GET /v1/server` : 256 Kio par contenu chiffré, 500 changements et 4 Mio par envoi, 21 Mio par paquet chiffré (20 Mio de paquet, plus le complément).
- **Débit :** au-delà, le serveur répond `429` avec `Retry-After`. La limite couvre toutes les routes qui vérifient `authKey` ou `recoveryAuthKey` : `login`, `recover/start`, `account/password`, `account/recovery`, `account/email` et `account/delete`. Sinon, un jeton volé permettrait de deviner le mot de passe en ligne, par les routes du compte. Conseillé, toutes routes confondues : 10 tentatives par quart d'heure pour un même couple (adresse IP, compte), et 100 par heure pour un même compte, toutes adresses IP confondues. Une limite par compte aussi basse que la première permettrait à n'importe qui de bloquer la connexion d'un compte dont il connaît l'adresse e-mail : peu grave sur un serveur privé, mais à éviter sur le serveur public.
- **Compatibilité :** des champs peuvent s'ajouter en v1, et le client ignore les champs inconnus. Tout changement incompatible passe par `/v2`.

### 4.2 Serveur

#### `GET /v1/server`

Sans authentification.

```json
{
  "api": 1,
  "software": "syncserver 0.1.0",
  "registration": "invitation",
  "limits": {
    "maxPayloadBytes": 262144,
    "maxChangesPerPush": 500,
    "maxRequestBytes": 4194304,
    "maxBlobBytes": 22020096,
    "maxAccountBytes": null
  },
  "minimumIterations": 600000,
  "maximumIterations": 10000000,
  "features": ["events", "blobs"]
}
```

- `minimumIterations` et `maximumIterations` : bornes que le serveur impose aux paramètres `kdf`. Le client applique toujours ses propres bornes (§ 3.1), même si le serveur en annonce de plus larges.
- `registration` : `open`, `invitation` (code donné par l'administrateur) ou `closed`.
- `features` : contient `events` si le serveur fournit le flux d'événements (§ 4.6), et `blobs` s'il garde les paquets d'extension (§ 4.7). Sans `blobs`, les extensions installées depuis un fichier voyagent sans leur paquet : l'autre Mac les montre « à réinstaller ».

### 4.3 Inscription et sessions

Les réponses de connexion renvoient toutes un objet **Session** :

```json
{
  "accountId": "5D0C7A3E-2B1F-4E8D-9A6C-0F3B7E1D2C54",
  "deviceId": "3F2A8C1E-6B0D-4C5E-9F1A-2B7D8E4C6A10",
  "accessToken": "…",
  "refreshToken": "…",
  "expiresIn": 900,
  "wrappedAccountKey": "…",
  "keyVersion": 1
}
```

#### `POST /v1/auth/prelogin`

```json
{ "email": "jeremy@example.com" }
```

Réponse `200` :

```json
{ "kdf": { "algorithm": "pbkdf2-sha256", "iterations": 600000, "salt": "q83vEjRWeJq83vEjRWeJqw==" } }
```

Pour un identifiant inconnu, le serveur répond avec des paramètres plausibles et stables : le sel est HMAC-SHA256(secret du serveur, identifiant), tronqué à 16 octets, et `iterations` vaut la valeur par défaut. `prelogin` ne révèle donc pas quels comptes existent. L'inscription, elle, le révèle (`email_taken`, § 3.9). Le secret du serveur est décrit au § 4.9.

#### `POST /v1/accounts`

```json
{
  "email": "jeremy@example.com",
  "invitation": "K7QD-2MXP",
  "kdf": { "algorithm": "pbkdf2-sha256", "iterations": 600000, "salt": "q83vEjRWeJq83vEjRWeJqw==" },
  "authKey": "…",
  "wrappedAccountKey": "…",
  "recovery": { "authKey": "…", "wrappedAccountKey": "…" },
  "device": { "id": "3F2A8C1E-6B0D-4C5E-9F1A-2B7D8E4C6A10", "name": "MacBook Pro" }
}
```

Réponse `201` : Session. `invitation` n'est requis que si `registration` vaut `invitation`.
Erreurs : `registration_closed`, `invitation_required`, `email_taken`, `weak_kdf`.

#### `POST /v1/auth/login`

```json
{ "email": "jeremy@example.com", "authKey": "…", "device": { "id": "…", "name": "Mac mini" } }
```

Réponse `200` : Session. Erreur : `invalid_credentials`.

Si `device.id` est déjà connu pour ce compte (reconnexion après une révocation, § 5), le serveur réactive cet appareil et lui donne de nouveaux jetons ; les anciens restent révoqués. Un appareil est identifié par le couple (compte, `device.id`) : un Mac qui change de compte garde son `device.id`.

#### `POST /v1/auth/refresh`

```json
{ "refreshToken": "…" }
```

Réponse `200` : `{ "accessToken": "…", "refreshToken": "…", "expiresIn": 900 }`. Erreurs : `invalid_token`, `device_revoked`, `account_deleted`.

#### `POST /v1/auth/logout`

Authentifié : en-tête `Authorization` avec le jeton d'accès, sans corps. Réponse `204`. Révoque les jetons de cet appareil.

#### `POST /v1/auth/recover/start`

```json
{ "email": "jeremy@example.com", "recoveryAuthKey": "…" }
```

Réponse `200` : `{ "recoveryToken": "…", "expiresIn": 600, "recoveryWrappedAccountKey": "…" }`. Erreur : `invalid_credentials`.

#### `POST /v1/auth/recover/finish`

```json
{
  "recoveryToken": "…",
  "kdf": { "algorithm": "pbkdf2-sha256", "iterations": 600000, "salt": "…" },
  "authKey": "…",
  "wrappedAccountKey": "…",
  "recovery": { "authKey": "…", "wrappedAccountKey": "…" },
  "device": { "id": "…", "name": "MacBook Pro" }
}
```

Réponse `200` : Session. Tous les autres appareils sont révoqués. `recovery` est facultatif : s'il est fourni, il remplace la clé de secours.
Erreurs : `invalid_token`, `weak_kdf`.

### 4.4 Compte

Toutes les routes de cette section sont authentifiées.

| Route | Corps | Réponse |
| --- | --- | --- |
| `GET /v1/account` | — | `200` `{ "accountId", "email", "createdAt", "usage": { "records", "blobs", "bytes" }, "maxAccountBytes" }` (`bytes` compte enregistrements et paquets) |
| `POST /v1/account/password` | `{ "authKey", "kdf", "newAuthKey", "wrappedAccountKey" }` | `204` ; les autres appareils sont révoqués |
| `POST /v1/account/recovery` | `{ "authKey", "recovery": { "authKey", "wrappedAccountKey" } }` | `204` ; nouvelle clé de secours |
| `POST /v1/account/email` | `{ "authKey", "email" }` | `204` ; nouvel identifiant, normalisé comme au § 4.1. Erreur `email_taken`. Les appareils restent connectés. Le client propose d'enregistrer un nouveau kit de secours, puisque l'identifiant y figure |
| `POST /v1/account/delete` | `{ "authKey" }` | `204` ; compte, appareils, enregistrements et paquets effacés tout de suite. Le serveur garde seulement, pendant 60 jours, les empreintes des jetons des appareils du compte, pour leur répondre `account_deleted` |

Erreurs : `invalid_credentials` (mauvaise `authKey`), `weak_kdf`, `email_taken`. Les codes sont décrits au § 4.8.

### 4.5 Appareils

Toutes les routes de cette section sont authentifiées.

| Route | Corps | Réponse |
| --- | --- | --- |
| `GET /v1/devices` | — | `200` `{ "devices": [{ "id", "name", "createdAt", "lastSeenAt", "current" }] }` |
| `PATCH /v1/devices/{id}` | `{ "name" }` | `200` : l'appareil modifié |
| `DELETE /v1/devices/{id}` | — | `204` ; ses jetons sont révoqués. L'appareil reçoit alors `device_revoked` et garde ses données locales. |

### 4.6 Synchronisation

Toutes les routes de cette section sont authentifiées. Chaque compte a un compteur, qui augmente à chaque écriture ; la `version` d'un enregistrement est la valeur du compteur à sa dernière écriture.

Un **enregistrement**, tel que le serveur le renvoie :

```json
{
  "id": "7C4B2E9A-1D3F-4B6E-8A0C-5E2D9F1B3A76",
  "version": 1843,
  "deleted": false,
  "keyVersion": 1,
  "payload": "AQ…",
  "updatedAt": "2026-10-17T08:30:12.401Z"
}
```

Une suppression garde aussi son `payload` chiffré (§ 2.2, `deleted: true`). C'est lui qui fait foi pour le client.

#### `GET /v1/sync/changes?since=<cursor>&limit=<n>`

Renvoie les enregistrements écrits après `cursor`, par version croissante, au plus `limit` (500 par défaut, 1 000 au maximum). Sans `since`, la lecture part du début (première synchro).

```json
{ "changes": [ … ], "cursor": "c_1843", "hasMore": false }
```

- `cursor` est opaque. Le client le garde une fois le lot appliqué, et relit tant que `hasMore` vaut `true`.
- Les suppressions sont conservées au moins 180 jours. Un curseur trop ancien, ou d'une autre époque (restauration d'une sauvegarde), reçoit `410 cursor_expired` (règle exacte au § 4.9) ; le client relit alors tout depuis le début (§ 5).

#### `POST /v1/sync/push`

En-tête `Idempotency-Key: <UUID>`. Si la même clé revient dans les 24 heures, le serveur renvoie la même réponse sans réécrire (portée de la clé : § 4.9).

Côté client, un lot et sa clé sont écrits dans `SyncState` avant l'envoi, et y restent jusqu'à la réponse. Après une erreur réseau, un délai dépassé ou un redémarrage, le client renvoie **exactement** le même lot avec la même clé, sans le recomposer, même si d'autres changements sont arrivés entre-temps. Ces changements partent dans le lot suivant. Sinon, un lot déjà appliqué par le serveur, mais dont la réponse s'est perdue, serait renvoyé sous une autre clé. Il serait alors pris en conflit avec lui-même.

```json
{
  "changes": [
    { "id": "7C4B2E9A-…", "baseVersion": 1842, "deleted": false, "keyVersion": 1, "payload": "AQ…" },
    { "id": "E1A0F6B2-…", "baseVersion": null, "deleted": false, "keyVersion": 1, "payload": "AQ…" }
  ]
}
```

- `baseVersion` est la version sur laquelle repose le changement, ou `null` pour une création.
- Les changements sont traités un par un, dans l'ordre, chacun de façon atomique. Un changement est **appliqué** si `baseVersion` est égal à la version actuelle (ou vaut `null` pour un identifiant inconnu). Sinon, il est **en conflit** et rien n'est écrit pour lui.
- **Identifiant inconnu du serveur avec un `baseVersion`** (jamais écrit, ou suppression purgée après 180 jours) : c'est un conflit avec `"current": null`. Le client renvoie alors son changement comme une création (`baseVersion: null`). Si c'était une suppression, il n'y a rien à renvoyer : le serveur n'a déjà plus l'enregistrement, et le client le note comme fait.

Réponse `200` :

```json
{
  "results": [
    { "id": "7C4B2E9A-…", "status": "applied", "version": 1843 },
    { "id": "E1A0F6B2-…", "status": "conflict", "current": { "id": "E1A0F6B2-…", "version": 1790, "…": "…" } },
    { "id": "4F8A1C3D-…", "status": "conflict", "current": null },
    { "id": "0B9D3C7E-…", "status": "rejected", "code": "payload_too_large" }
  ]
}
```

Codes d'un changement `rejected`, et réaction du client :

- `payload_too_large` : l'enregistrement reste sur ce Mac sans partir, et la synchro affiche un avertissement qui le nomme (par exemple « Le Space “Travail” est trop gros pour être synchronisé »). Il ne repart que s'il change (§ 5).
- `quota_exceeded` : rien n'est perdu. Le changement reste en attente, et la synchro prévient que l'espace du compte est plein.
- `invalid_payload` : bug, journalisé ; le changement ne repart que s'il change.

Erreurs sur la requête entière : `request_too_large`, `invalid_request`.

**Quota plein :** dès qu'un envoi reçoit `quota_exceeded`, le client envoie d'abord ses suppressions, dans un lot à part, puisqu'elles libèrent de la place. Les autres changements suivent ensuite. Une suppression n'est jamais refusée pour cause de quota.

#### `GET /v1/sync/events`

Disponible si `features` contient `events`. Le flux est en `text/event-stream` (Server-Sent Events). Chaque fois qu'un autre appareil du compte a fait appliquer des changements, le serveur envoie :

```
event: changes
data: {"version":1843}

```

Il envoie aussi un commentaire `: ping` toutes les 30 secondes. À chaque événement, le client relit les changements.

Le serveur ferme le flux quand le jeton d'accès qui l'a ouvert expire. Il envoie juste avant `event: expired` (sans `data`). Cette fermeture n'est pas une panne : le client renouvelle le jeton (§ 4.1, un seul renouvellement à la fois) et se reconnecte tout de suite. Il relit aussi les changements, pour ne rien manquer entre les deux connexions. Si la connexion tombe autrement (erreur réseau, réponse autre que `200`, plus de `ping` depuis 90 secondes), il se reconnecte avec un délai croissant, de 1 seconde à 5 minutes.

### 4.7 Paquets d'extension

Disponible si `features` contient `blobs`. Toutes les routes sont authentifiées. Un paquet est un contenu binaire opaque, chiffré par le client (§ 3.5), qui ne change jamais : une nouvelle version d'un paquet a un nouvel identifiant.

| Route | Corps | Réponse |
| --- | --- | --- |
| `PUT /v1/blobs/{id}` | binaire (`application/octet-stream`), en-tête `If-None-Match: *` | `201` ; `200` si le même contenu existe déjà sous cet identifiant ; `409 blob_exists` s'il en existe un autre |
| `GET /v1/blobs/{id}` | — | `200` binaire ; `404 not_found` |
| `DELETE /v1/blobs/{id}` | — | `204`, même s'il n'existait pas |

- Le serveur ne sait pas quels paquets servent encore : c'est le client qui supprime l'ancien paquet d'une extension mise à jour ou désinstallée, après avoir fait appliquer l'enregistrement qui ne le cite plus.
- Ordre d'envoi : le paquet d'abord, puis l'enregistrement qui le cite.
- **Paquet introuvable (`404`)** : l'enregistrement peut arriver avant son paquet, ou citer un paquet perdu. Le client réessaie avec un délai croissant (1 minute, doublé à chaque essai, au plus 6 heures), 10 fois au plus. Le nombre d'essais est gardé dans `SyncState`. Ensuite, l'extension est montrée « à réinstaller », avec un bouton « Réessayer ». Un nouvel enregistrement qui cite un autre paquet remet le compte à zéro.
- **Paquets orphelins** : avant d'envoyer un paquet, le client note son identifiant dans `SyncState` (« en attente »). Il retire cette note quand l'enregistrement qui le cite est appliqué. Il supprime le paquet (`DELETE`) si l'envoi de l'enregistrement échoue pour de bon ou si le conflit est perdu. Au lancement, il supprime aussi les paquets restés « en attente » sans enregistrement en cours d'envoi.
- Les paquets comptent dans `maxAccountBytes`. Au-delà : `507 quota_exceeded`. Trop gros : `413 blob_too_large`.

### 4.8 Codes d'erreur

| `code` | HTTP | Sens, et réaction du client |
| --- | --- | --- |
| `invalid_request` | 400 | Requête mal formée : bug, journalisé |
| `weak_kdf` | 400 | Paramètres `kdf` hors des bornes (algorithme inconnu, itérations hors de `minimumIterations` … `maximumIterations`, sel qui ne fait pas 16 octets) |
| `invalid_credentials` | 401 | Identifiant, `authKey` ou clé de secours faux |
| `token_expired` | 401 | Jeton d'accès expiré : le renouveler (un seul renouvellement à la fois), puis rejouer la requête |
| `invalid_token` | 401 | Jeton inconnu : demander une nouvelle connexion |
| `device_revoked` | 401 | Appareil révoqué : se déconnecter et garder les données locales (§ 5) |
| `account_deleted` | 401 | Compte supprimé depuis un autre appareil : se déconnecter, garder les données locales, et dire « Ce compte de synchronisation a été supprimé depuis un autre appareil » |
| `registration_closed` | 403 | Inscriptions fermées |
| `invitation_required` | 403 | Invitation absente, invalide ou déjà utilisée |
| `not_found` | 404 | Ressource inconnue |
| `email_taken` | 409 | Identifiant déjà utilisé |
| `blob_exists` | 409 | Un autre paquet porte déjà cet identifiant : bug, journalisé |
| `cursor_expired` | 410 | Curseur trop ancien : tout relire depuis le début |
| `request_too_large` | 413 | Envoi trop gros : le découper |
| `payload_too_large` | — | Changement refusé dans une réponse `200` de `/v1/sync/push` : l'enregistrement reste local, avec un avertissement (§ 4.6) |
| `invalid_payload` | — | Changement refusé dans une réponse `200` de `/v1/sync/push` : bug, journalisé (§ 4.6) |
| `blob_too_large` | 413 | Paquet au-delà de `maxBlobBytes` : l'extension voyage sans son paquet |
| `unsupported_client` | 426 | Version de Matcha trop ancienne : proposer la mise à jour |
| `rate_limited` | 429 | Trop de requêtes : attendre `Retry-After` |
| `server_error` | 500 | Erreur du serveur : réessayer plus tard |
| `quota_exceeded` | 507 | Espace du compte plein : prévenir l'utilisateur, envoyer d'abord les suppressions (§ 4.6) ; aussi code d'un changement refusé |
| `unavailable` | 503 | Maintenance : attendre `Retry-After` |

### 4.9 Exigences pour le serveur

- **Écritures d'un compte, une à la fois.** Le serveur traite l'une après l'autre les écritures d'un même compte : envois, paquets, opérations sur le compte. Selon la base :
  - SQLite : une transaction `BEGIN IMMEDIATE` par requête, ou un verrou par compte dans le serveur ;
  - PostgreSQL : un `SELECT … FOR UPDATE` sur la ligne du compte, ou `pg_advisory_xact_lock` sur son identifiant, au début de la transaction.

  Le verrou n'est jamais tenu pendant la réception d'un corps de requête. Un paquet, qui peut faire 21 Mio, est d'abord reçu dans un fichier temporaire, et sa taille vérifiée. Le serveur ne prend le verrou que pour l'écriture finale : quota, ligne en base, fichier mis en place. Sinon, tous les envois du compte attendraient la fin d'un téléchargement. Une version n'est visible dans `/v1/sync/changes` qu'une fois écrite, et jamais avant une version plus petite. Sinon, un client avancerait son curseur au-delà d'un changement encore en cours d'écriture, et ne le verrait jamais.
- **Dernière version seulement.** Le serveur ne garde, pour chaque enregistrement, que sa dernière version. Une suppression garde son `payload` chiffré (§ 4.6) au moins 180 jours, puis peut être purgée.
- **Versions et curseur.** La `version` augmente strictement, et ne revient jamais en arrière, même après la restauration d'une sauvegarde. Le plus simple est de prendre max(dernière version + 1, heure du serveur en microsecondes). Le curseur, opaque pour le client, contient une **époque** et une version. L'époque est un identifiant tiré à la création de la base et tiré de nouveau à chaque restauration d'une sauvegarde. Le serveur détecte seul une restauration. Il tient, hors de la base, un fichier repère qui contient la plus grande version écrite, mis à jour après chaque écriture validée. Au démarrage, si la plus grande version de la base est plus petite que celle du repère, la base a été restaurée : le serveur tire une nouvelle époque et met le repère à jour. Si le repère manque, il est recréé à partir de la base, sans changer d'époque. Le repère ne doit pas faire partie des sauvegardes. S'il a été restauré avec la base, la commande d'administration `syncserver rotate-epoch` tire une nouvelle époque à la main. Le serveur garde la plus grande version purgée. Un curseur répond `410 cursor_expired` dans deux cas : son époque n'est pas celle de la base, ou sa version est plus petite que la plus grande version purgée. Le client relit alors tout (§ 5), et renvoie ce que la sauvegarde avait perdu.
- **Limites, en octets.** `maxPayloadBytes` compte le `payload` décodé (binaire) ; `maxRequestBytes` compte le corps HTTP tel qu'il arrive, en JSON et base 64 ; `maxBlobBytes` compte le corps binaire d'un paquet. `maxAccountBytes` compte les `payload` décodés de tous les enregistrements, suppressions comprises, plus les paquets.
- **`Idempotency-Key`.** Sa portée est le couple (compte, appareil). Le serveur garde la réponse 24 heures, avec une empreinte du corps. La même clé avec un corps différent reçoit `400 invalid_request`, sans rien écrire. Une requête en cours avec la même clé fait attendre la seconde, qui reçoit ensuite la même réponse.
- **Hachage des clés reçues.** `authKey` et `recoveryAuthKey` sont hachées avec Argon2id, au minimum m = 19 Mio, t = 2, p = 1 (recommandation de l'OWASP) ; conseillé : m = 64 Mio, t = 3, p = 4 (RFC 9106). Les paramètres sont gardés avec chaque hachage (format PHC), pour pouvoir les relever : le hachage est refait à la connexion suivante.
- **Secret de `prelogin`** (§ 4.3) : 32 octets aléatoires, tirés au premier démarrage et gardés avec les données, jamais dans la configuration ni dans les journaux. Il ne change jamais, et survit à une restauration : en changer modifierait les sels inventés pour les identifiants inconnus, ce qui révélerait qu'ils l'étaient.
- **Temps de réponse constant.** `prelogin`, `login` et `recover/start` répondent dans le même temps, que le compte existe ou non. `prelogin` calcule le sel factice dans tous les cas. Pour un compte inconnu, `login` et `recover/start` vérifient la clé reçue contre un hachage Argon2id factice et répondent la même erreur (`invalid_credentials`). Les comparaisons de jetons et d'empreintes se font en temps constant.
- **Jetons.** Le serveur ne garde que leur empreinte (SHA-256). Un jeton de renouvellement déjà utilisé, présenté de nouveau, révoque l'appareil (§ 4.1).
- **Révocation immédiate.** Le jeton d'accès est vérifié en base à chaque requête, pas seulement à sa création : un appareil révoqué, ou un compte supprimé, est refusé dès la requête suivante. Le serveur ferme aussi tout de suite les flux d'événements ouverts concernés :
  - ceux d'un appareil révoqué ;
  - ceux d'un compte supprimé ;
  - ceux des autres appareils, après un changement de mot de passe ou une récupération.

  Il ferme ces flux sans `event: expired`. Le client se reconnecte, puis reçoit `device_revoked` ou `account_deleted` (§ 4.8). Sans cela, ces flux continueraient de recevoir des événements jusqu'à 15 minutes.
- **Invitations.** Un code de 8 caractères en base 32 de Crockford, en 2 groupes de 4 (`K7QD-2MXP`). À la saisie, il se normalise comme une clé de secours (§ 3.2). Il n'a qu'un usage, et expire au bout de 7 jours par défaut. Le serveur n'en garde que l'empreinte. La commande d'administration crée, liste et révoque les invitations. `invitation_required` répond aussi bien à un code absent qu'à un code faux, expiré ou déjà utilisé.
- **Journaux.** Le serveur ne journalise jamais `authKey`, `recoveryAuthKey`, les enveloppes, les jetons, les codes d'invitation, ni aucun `payload` ou paquet. L'adresse e-mail n'apparaît que tronquée ou hachée. Les en-têtes `Authorization` et `Idempotency-Key` sont masqués.
- **Flux d'événements derrière un proxy.** Le serveur répond avec `Content-Type: text/event-stream`, `Cache-Control: no-cache` et `X-Accel-Buffering: no`, pour que nginx ne retienne pas les événements. Il vide son tampon après chaque événement. Le délai d'inactivité du proxy doit dépasser 30 secondes, l'intervalle des `ping`.

---

## 5. Déroulement côté client

**Déclencheurs :** une modification locale (regroupée sur 2 secondes ; 5 secondes de stabilité pour l'adresse d'un onglet du jour, § 2.4), le lancement, le retour au premier plan, un événement du serveur, une lecture toutes les 15 minutes sans flux d'événements, et le bouton « Synchroniser maintenant ». Une seule synchro tourne à la fois.

**Changements locaux :** pour tous les types, une comparaison, enregistrement par enregistrement, de l'état actuel de ce Mac avec celui de sa dernière synchro (ce qu'il a envoyé, ou ce qu'il avait juste après avoir appliqué un changement reçu). Un changement reçu n'est donc jamais renvoyé en écho, et un enregistrement que ce Mac garde tel quel (onglet affiché) ne repart pas. Un enregistrement en attente de son parent n'est pas pris pour une suppression ; un contenu d'un format plus récent, d'un type inconnu ou illisible n'est jamais modifié ici (§ 2.2, § 2.9) ; un changement refusé par le serveur ne repart que s'il change.

**Valeurs reçues mais pas appliquées.** Certaines valeurs reçues ne sont pas appliquées sur ce Mac :
- une autorisation `allow` (§ 2.6) ;
- la `version` et la `source` d'une mise à jour d'extension pas encore installée, les droits d'une extension, l'accès aux fenêtres privées (§ 2.5) ;
- la nouvelle adresse d'un onglet affiché (§ 2.4).

Pour chaque enregistrement, `SyncState` garde donc deux choses : le dernier contenu accepté, et l'état que ce Mac en a tiré. Les changements locaux se calculent par rapport à cet état, champ par champ, et entrée par entrée pour les listes de droits. Le contenu envoyé reprend le dernier contenu accepté, où seuls les champs changés sur ce Mac sont remplacés. Une valeur reçue et non appliquée repart donc telle quelle. Un enregistrement que ce Mac n'a pas appliqué du tout (un `allow` reçu, par exemple) n'est jamais pris pour une suppression locale : sa suppression ne part que si l'utilisateur le supprime ici.

**Enregistrements locaux.** Un favori ou un épinglé dont l'adresse n'est pas web (§ 2.4) ne voyage pas. Le moteur ne l'envoie pas, n'envoie pas sa suppression, et ne le prend pas pour une absence : « Remplacer les données de ce Mac » et la première synchro le gardent. S'il avait déjà voyagé avant de prendre son adresse locale, les autres Mac gardent la dernière version envoyée. Une modification reçue ensuite s'y applique, sauf pour l'adresse et le titre. Il repart quand il retrouve une adresse web.

**Garde-fou contre les suppressions en masse :** une lecture de la base locale en échec est une erreur, jamais un état vide : le cycle s'arrête sans rien envoyer. Si un cycle s'apprête à envoyer la suppression de plus de la moitié des enregistrements du compte (et d'au moins 50), le client s'arrête et demande confirmation, en disant ce qui serait supprimé sur tous les Mac. « Remplacer les données synchronisées » en est exempté : l'utilisateur vient de le confirmer.

**Un cycle :**
1. **Envoyer** les changements en attente, par lots. Un lot resté sans réponse part d'abord, tel quel, avec sa clé (§ 4.6). Un nouveau paquet d'extension part avant l'enregistrement qui le cite (§ 4.7). Pour chaque conflit, déchiffrer `current` et appliquer la règle du § 2.3 (`current: null` : § 4.6). Si `current` porte le même couple (`modifiedAt`, `modifiedBy`) que le changement local, ce changement est déjà appliqué : par exemple, un lot renvoyé plus de 24 heures après son premier envoi, quand le serveur a oublié sa clé. Le client prend alors `current.version` et ne renvoie rien. Si le serveur a la version la plus récente, elle est appliquée en local et le changement local est abandonné. Sinon, le changement local repart avec `baseVersion` = `current.version`.
2. **Lire** les changements depuis le curseur jusqu'à `hasMore: false`. Déchiffrer chaque contenu, le vérifier, l'appliquer (§ 2.3) face aux changements locaux encore en attente.
3. **Garder** le curseur et l'état de chaque enregistrement (`SyncState`, dans Application Support), après chaque lot : un cycle interrompu reprend où il en était.

**Première synchro, ou changement de mode :**
- Si les données synchronisées sont vides, le client envoie tout.
- Si ce Mac n'a que les données du premier lancement (profil « Personnel », Space « Perso », aucun favori ni épinglé, au plus un onglet du jour vide), il prend les données synchronisées et retire les siennes, comme avec « Remplacer les données de ce Mac ». Sans ça, chaque Mac ajouterait son propre « Perso ».
- Sinon, il demande : **Fusionner** (par défaut : union par identifiant, règle du § 2.3), **Remplacer les données de ce Mac**, ou **Remplacer les données synchronisées**.
- En fusion, un enregistrement présent des deux côtés prend la valeur synchronisée : pour les réglages, les décisions de site et l'état des extensions, ce Mac n'ajoute que ce qui n'y est pas encore. Sinon le dernier Mac branché imposerait ses réglages à tous. Les extensions des autres Mac arrivent sous « Depuis vos autres Mac » (§ 2.5).
- « Remplacer les données de ce Mac » retire de ce Mac les profils, Spaces, dossiers, onglets, décisions de site et zooms absents des données synchronisées, sauf les enregistrements locaux (favoris et épinglés à adresse non web, ci-dessus) : ils restent tant que leur Space ou leur profil reste, et un dossier qui disparaît les fait remonter à la racine (§ 2.3). Ses réglages et extensions qui n'y sont pas restent sur ce Mac, sans partir : ils partiront à leur prochaine modification.
- « Remplacer les données synchronisées » envoie la suppression de ce qui n'est pas sur ce Mac, puis tout ce qu'il a, daté après tout ce qui a été lu. Il ne touche pas aux enregistrements d'un format plus récent, d'un type inconnu ou illisibles (§ 2.2, § 2.9) : ils restent sur le serveur tels quels.

**Curseur expiré :** le client relit tout depuis le début, en appliquant les règles habituelles (§ 2.3). Il n'efface rien sur la foi d'une absence. Sinon, un serveur, même sans les clés, pourrait effacer profils, extensions, autorisations et réglages sur tous les Mac en « oubliant » des enregistrements. À la place, il renvoie deux sortes d'enregistrements :
- ceux de ce Mac absents de la relecture, comme des créations ;
- ceux dont le serveur n'a qu'une version plus ancienne que la dernière acceptée ici (serveur restauré d'une sauvegarde), avec ce dernier contenu.

S'il s'apprête ainsi à recréer plus de la moitié des enregistrements du compte (et au moins 50), il demande d'abord : « Le serveur ne connaît plus N éléments de ce Mac ». Deux choix : « Les renvoyer » (par défaut) ou « Remplacer les données de ce Mac ». En contrepartie, un Mac resté plus de 180 jours sans synchro peut faire revenir ce qui a été supprimé ailleurs entre-temps.

**Appareil révoqué, compte supprimé ou déconnexion :** la synchro s'arrête et les données de navigation de ce Mac restent. Les jetons et la clé de compte sont retirés du trousseau ; l'identifiant de l'appareil est gardé. Le sort de `SyncState` (curseur, état des enregistrements, changements en attente) dépend de la cause :
- `device_revoked` (par exemple après un changement de mot de passe sur un autre Mac) : il est gardé, pour reprendre à la reconnexion. Message : « Ce Mac a été déconnecté de la synchronisation ».
- `account_deleted` : il est effacé, car il ne servira plus. Message : « Ce compte de synchronisation a été supprimé depuis un autre appareil ».
- Déconnexion choisie par l'utilisateur (« Se déconnecter », synchro désactivée) : il est effacé. Ce fichier copie en clair une partie des données ; une reconnexion plus tard sera une première synchro.

**Reconnexion :**
- même serveur et même `accountId`, avec un `SyncState` gardé : la synchro reprend là où elle en était, avec son curseur et ses changements en attente, sans l'écran « Fusionner » ;
- autre compte ou autre serveur : c'est une première synchro. L'état gardé pour l'ancien compte est mis de côté (un fichier `SyncState` par serveur et par compte).

Un `SyncState` qui ne sert plus est une copie en clair d'une partie des données. C'est le cas de l'état d'un ancien compte, de celui d'un Mac déconnecté par `device_revoked` qui ne s'est pas reconnecté, ou d'un fichier d'état illisible mis de côté (ci-dessous). Il est effacé au bout de 60 jours sans reconnexion, la durée d'un jeton de renouvellement.

**Mac cloné** (Assistant de migration, restauration de Time Machine) : la copie aurait le même identifiant d'appareil et les mêmes jetons que l'original. Le premier des deux qui réutiliserait le jeton de renouvellement ferait révoquer l'appareil. Le client garde donc, à côté de l'identifiant de l'appareil, une marque de la machine : SHA-256 de `IOPlatformUUID` (IOKit, `kIOPlatformUUIDKey` ; API publique, à vérifier dans le bac à sable à la tâche 7.7). Au lancement, si la marque a changé, ce Mac est une copie :
- il tire un nouvel identifiant d'appareil ;
- il efface ses jetons sans les utiliser ;
- il demande une nouvelle connexion, qui reprend `SyncState` comme une reconnexion au même compte.

**Fichier d'état illisible :** le fichier est mis de côté (renommé), et non écrasé. L'identifiant de l'appareil est relu dans le trousseau. La synchro repart comme une première synchro avec le même compte, en « Fusionner » sans le demander : relecture complète, et un enregistrement présent des deux côtés prend la valeur synchronisée.

---

## 6. iCloud

Reporté le 2026-10-18 (`FEATURES.md`) : la synchro passe d'abord par un serveur. Ce qui suit reste la spécification pour la reprise.

- Moteur : `CKSyncEngine`, sur la base privée du conteneur `iCloud.com.matchabrowser.matcha` (tâche 7.3), zone `Matcha`.
- Un `CKRecord` de type `Item` par enregistrement. Son `recordName` est l'`id`. Le contenu JSON du § 2.2 va dans `encryptedValues["payload"]`, sans complément ni chiffrement par Matcha : CloudKit le chiffre avec des clés du trousseau iCloud.
- Conflit (`serverRecordChanged`) : même règle qu'au § 2.3, à partir de l'enregistrement renvoyé par CloudKit.
- L'état du moteur (`CKSyncEngine.State.Serialization`) est gardé dans Application Support.
- Changement ou déconnexion du compte iCloud : la synchro s'arrête, les données locales restent, et les réglages affichent l'état.
- Paquet d'extension : un `CKRecord` de type `Blob` (son `recordName` est le `blobId`) avec un `CKAsset`, le fichier du paquet chiffré comme pour un serveur (§ 3.5). `encryptedValues` ne couvre pas les fichiers : c'est la clé du paquet, dans l'enregistrement de l'extension, qui le protège.
- Pas de mot de passe maître : la protection est celle du compte Apple.
- Identifiants tirés d'une clé (§ 2.2) : sans `accountKey`, `idKey` est à définir à la reprise, par exemple une clé aléatoire gardée dans un enregistrement de la base privée.

---

## 7. Serveur public

Même API, à une adresse fixée dans l'app (à définir), avec `registration: open`. Avant de l'ouvrir, il faudra décider de l'hébergement, des quotas (`maxAccountBytes`), de la vérification des adresses e-mail (routes à ajouter en v1), des conditions d'utilisation et de la procédure de suppression des comptes. En attendant, l'option reste grisée dans les réglages, avec la mention « Bientôt disponible ».

---

## 8. Questions ouvertes

- **Argon2id plutôt que PBKDF2** côté client, pour mieux résister aux essais hors ligne. Il faudrait une bibliothèque (libsodium ou équivalent), donc ton accord. Le champ `kdf.algorithm` permet de migrer plus tard, compte par compte.
- **Protection contre un serveur qui cache des versions :** un serveur ne peut plus rejouer un ancien contenu à un Mac qui l'a déjà reçu (§ 2.3), mais il peut encore cacher des enregistrements ou des suppressions, et servir ce qu'il veut à un Mac sans état. Un journal signé des versions, en v2.
- **Synchroniser plus :** historique de navigation, split views, exceptions du bloqueur intégré s'il revient. Plus tard, et au choix de l'utilisateur.
- **Choix par type de données :** une case par type dans les réglages (onglets du jour, extensions, réglages) ? Pas en v1 : tout ce qui est listé au § 2.1 voyage.