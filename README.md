# Mace Club Heemskerk

Go-website met Google-login, een mediabibliotheek in PostgreSQL (productie) of SQLite (lokaal) en trainingsfoto’s. Geen JavaScript-build of lokale Go-installatie nodig. Docker compileert de Go-code; de runtime bevat de binary, CA-certificaten en persistente data onder `/data`.

## Lokaal starten

Vanuit deze projectmap, met Docker gestart:

```sh
docker compose up --build -d web
```

Open http://localhost:18080. Na een wijziging voer je hetzelfde commando opnieuw uit: HTML en CSS zitten in de Go-binary en worden bij het bouwen bijgewerkt.

```sh
docker compose logs -f web
docker compose down
```

## Go gebruiken zonder lokale compiler

De `tools`-service voert Go uit in een container met de projectmap gekoppeld:

```sh
docker compose run --rm tools go fmt ./...
docker compose run --rm tools go vet ./...
docker compose run --rm tools go test ./...
```

Go-tests controleren Google-tokenverificatie, sessies, CSRF, toegangscontrole en caching. Daarnaast zijn er tests voor secrets-tooling en de JavaScript-versiecontrole:

```sh
docker compose run --rm tools go test ./...
node --test tests/version-monitor.test.cjs
python3 -m unittest discover -s tests -v
docker build --platform linux/amd64 -t maceclubheemskerk:verify .
python3 deploy/smoke-test.py maceclubheemskerk:verify
oc kustomize deploy
```

De smoketest gebruikt een eigen tijdelijke container met een willekeurige UID, alleen-lezen bestandssysteem en zonder Linux-capabilities. Hij controleert HTML, CSS, healthcheck, foutcodes en netjes stoppen. Je lokale website blijft draaien.

## Structuur

- `cmd/server`: opstarten, timeouts en netjes stoppen bij SIGTERM.
- `internal/web`: HTTP-routes en ingebouwde HTML/CSS/JavaScript.
- `internal/auth`: Google-verificatie, sessieopslag en beveiligde account-API.
- `GET /api/version`: fingerprint van de draaiende binary, inclusief alle ingebouwde assets.
- `deploy`: OpenShift Deployment, Service, Routes, image-pin en SealedSecrets-tooling.
- `.github/workflows/deploy.yml`: verificatie, image publiceren en GitOps image-pin.
- `GET /healthz`: status voor OpenShift-probes.
- `PORT`: luisterpoort, standaard `8080`.

`internal/content` gebruikt `database/sql`: pgx voor PostgreSQL in productie en de pure-Go `modernc.org/sqlite`-driver voor lokaal gebruik. `SQLITE_FILE` is standaard `/data/maceclub.sqlite`; `DATABASE_URL` selecteert PostgreSQL. Foto’s staan in `/data/uploads` (bij SQLite naast het databasebestand). De bestaande sessies blijven in `/data/sessions.json`; deze update maakt bestaande sessies niet ongeldig.

## Deployment naar OpenShift

Dezelfde keten als de andere apps: **GitHub Actions → GHCR → GitOps image-pin → Argo CD → OpenShift**.

- Pull requests voeren verificatie uit, zonder images te publiceren of productie aan te passen.
- Een push op `main` (of handmatige workflow op `main`) voert eerst de controles uit en bouwt daarna `linux/amd64`, de architectuur van het SNO-cluster.
- De image gaat naar `ghcr.io/robbertvdzon/maceclubheemskerk:sha-<commit>`.
- De workflow pint de exacte digest in `deploy/kustomization.yaml` en commit die met `[skip ci]`. Dit voorkomt een buildlus. Een achterhaalde build promoveert niet over nieuwere commits op main.
- Argo CD beheert namespace `maceclubheemskerk` met automatische sync, self-heal en prune. De Application staat in de sibling-repo `robberts-infrastructure`, onder `manifests/root-app/apps/maceclubheemskerk-application.yaml`.
- Er staan geen clustercredentials of databasewachtwoorden in GitHub Actions. De workflow gebruikt het ingebouwde `GITHUB_TOKEN` voor GHCR en de image-pin-commit.

### Eerste ingebruikname

1. Commit en push eerst de app-repo naar `main`. Wacht tot **Verify and deploy** is geslaagd en `newTag: bootstrap` in de Kustomization is vervangen door een echte digest. De bootstrap-tag is bewust geen bestaande image.
2. Zet het nieuwe GHCR-package in de package-instellingen op **Public**, zodat OpenShift het net als de bestaande publieke images zonder registrysecret kan ophalen. Een nieuw GHCR-package is standaard private, ook bij een publieke Git-repo; zie [GitHub Container registry](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry).
3. Commit en push daarna de nieuwe Application in `robberts-infrastructure`. De bestaande `root-apps`-Application maakt de app en namespace aan. Pas de appresources niet los handmatig toe: Git blijft de bron van waarheid.
4. Controleer de uitrol:

```sh
oc -n argocd get application maceclubheemskerk
oc -n maceclubheemskerk rollout status deployment/maceclubheemskerk --timeout=180s
oc -n maceclubheemskerk get pods,service,route
```

De workflow vraagt expliciet `contents: write` voor zijn image-pin-job. Eventuele branch protection moet die bot-commit toestaan; als main later alleen via PR's gewijzigd mag worden, moet ook de image-pin-stap naar een PR-flow worden omgezet.

### Domein koppelen

De site is ook bereikbaar via **https://maceclubheemskerk.vdzonsoftware.nl**. Deze extra OpenShift Route gebruikt de bestaande wildcard-DNS en Cloudflare Tunnel voor `*.vdzonsoftware.nl` en werkt onafhankelijk van de koppeling van het eigen domein.

Voeg `maceclubheemskerk.eu` toe aan hetzelfde Cloudflare-account als de gedeelde tunnel. Neem bestaande DNS/mailrecords over en stel de toegewezen Cloudflare-nameservers in bij one.com.

Voeg voor `maceclubheemskerk.eu` én `www.maceclubheemskerk.eu` een hostname met DNS-record toe aan de bestaande tunnel, met bestemming `http://router-internal-default.openshift-ingress.svc.cluster.local:80`. Behoud de oorspronkelijke Host-header. De bestaande wildcard voor `vdzonsoftware.nl` dekt deze namen niet.

Cloudflare verzorgt browser-TLS; de laatste verbinding binnen het cluster gebruikt HTTP. Daarom staat de OpenShift Route op `Allow`. Na activatie van HTTPS controleer je beide hostnamen. Er wordt geen extra tunnel aangemaakt.

### Secrets en ledenrechten

`secrets.example.env` is het lege voorbeeld in Git. De lokale `secrets.env` is aangemaakt met bestandsrechten `600` en wordt door Git én Docker genegeerd. Maak bij een nieuwe checkout zelf een kopie:

```sh
cp -n secrets.example.env secrets.env
chmod 600 secrets.env
```

`DATABASE_URL` selecteert PostgreSQL wanneer gevuld met een PostgreSQL-URL. Zonder die instelling gebruikt de app `SQLITE_FILE`, zonder databasewachtwoord. Productie gebruikt PostgreSQL; de CA en URL komen uit de deploymentconfiguratie en secrets. `GOOGLE_CLIENT_ID` configureert login; `MEMBER_EMAILS` bevat de komma-gescheiden Google-adressen van leden die content mogen toevoegen. Leeg betekent dat niemand mag toevoegen. Voeg toekomstige leden handmatig aan deze lijst toe; er is geen openbare registratie.

Zodra er echte waarden zijn ingevuld:

```sh
./deploy/seal-secrets.sh
```

Het script gebruikt `python3`, `kubeseal` en het publieke clustercertificaat uit `../robberts-infrastructure/manifests/cluster-bootstrap/cluster-cert.pem`. Afwijkende locaties kunnen via `--source` en `--cert`. Er is geen clusterlogin nodig voor het versleutelen.

- Env-inhoud wordt als data gelezen; shellcommando's en variabelen worden nooit uitgevoerd of uitgebreid. Eén `KEY=value` per regel; quotes zijn optioneel. Gebruik quotes als voor- of achterliggende spaties bij een waarde horen.
- Alleen ingevulde, toegestane keys worden versleuteld. Een leeg bestand, dubbele/ongeldige keys of een toolfout overschrijven geen bestaand secret.
- Plaintext gaat uitsluitend via stdin naar kubeseal, niet via commandoregelargumenten of tijdelijke bestanden.
- Het script schrijft `deploy/secrets/sealed-secret.json` en registreert dit in de bijbehorende Kustomization. Commit deze versleutelde bestanden; nooit `secrets.env`.
- De versleuteling is gebonden aan namespace `maceclubheemskerk` en Secret `maceclubheemskerk-secrets`; kopieer dit niet naar een andere omgeving.
- Argo CD synchroniseert het SealedSecret; de bestaande controller maakt het Secret. De Deployment leest dit via een optionele `envFrom`-referentie. De bestaande Reloader zorgt voor herstart bij secretwijzigingen.
- De lokale Compose-service leest `secrets.env` als ruwe waarden (geen shell- of dollar-expansie). De database-URL wordt alleen gebruikt wanneer PostgreSQL is ingesteld.

### Terugrollen

Herstel de vorige werkende image-digest in `deploy/kustomization.yaml` en commit die met `[skip ci]`. Argo CD synchroniseert terug naar die versie. Zo overschrijft een nieuwe imagebuild de rollback niet direct.

## Google-login en ingelogd blijven

De startpagina blijft openbaar. De knop **Inloggen** opent de officiële Google-inlogknop. Een ingelogde bezoeker ziet **Mijn account** en kan uitloggen. Alleen accounts uit `MEMBER_EMAILS` zien de toevoegknoppen en krijgen server-side schrijfrechten. Andere ingelogde accounts kunnen de openbare website bekijken, maar niets toevoegen.

De bestaande publieke Web OAuth-client-ID uit het project `tuinbewatering` (client `Robberts applicaties`) staat als `GOOGLE_CLIENT_ID` in het genegeerde `secrets.env` en in een SealedSecret. Er is geen Google client secret of Google refresh token nodig. Google geeft een kort geldig ID-token; de backend controleert de handtekening met de officiële Google Go-library en valideert audience, issuer, expiry, geverifieerde e-mail en eenmalige nonce. De gebruiker wordt geïdentificeerd met de stabiele Google `sub`.

### Eenmalig instellen bij Google

Voeg in **Google Cloud Console → Google Auth Platform → Clients → Robberts applicaties → Authorized JavaScript origins** toe:

- `https://maceclubheemskerk.eu`
- `https://www.maceclubheemskerk.eu`
- `https://maceclubheemskerk.vdzonsoftware.nl`
- `http://localhost:18080` voor lokaal testen

Voor deze popup-login zijn geen redirect-URI's nodig. Als het Google-project nog in Testing staat, moeten de gewenste accounts ook als test users zijn toegestaan. De Google-consoleconfiguratie en een echte login moeten door Robbert worden bevestigd; tests gebruiken lokaal getekende testtokens en geen productie-bypass.

### Sessies

- Eigen willekeurig sessietoken in een `HttpOnly`, `Secure`, `SameSite=Lax`, host-only cookie; geen login-token in localStorage. Op localhost gebruikt Compose expliciet een niet-Secure cookie; de server weigert die instelling voor publieke origins.
- Een sessie is 365 dagen geldig en wordt bij gebruik maximaal eenmaal per uur voor een jaar verlengd. De frontend controleert zijn sessie bij openen, terugkeren naar de tab en elk uur. Actieve bezoekers hoeven daardoor niet periodiek via Google in te loggen.
- Uitloggen trekt het token server-side in. Browsergegevens verwijderen, een jaar inactiviteit of verlies van de sessieopslag vereisen opnieuw inloggen. Sessies zijn per host: de twee eigen domeinnamen en het vdzonsoftware-adres delen geen cookie.
- `ALLOWED_EMAILS` kan een komma-gescheiden toegangslijst bevatten. Leeg betekent dat ieder geverifieerd Google-account mag inloggen; ook bestaande sessies worden bij iedere accountaanvraag opnieuw aan deze lijst getoetst.
- `SESSION_FILE` wijst naar een JSON-bestand met alleen tokenhashes, Google-gebruikersgegevens en vervaltijden. Het bestand wordt atomisch vervangen en is exclusief gelockt. Schrijffouten weigeren login; ze vallen niet terug naar tijdelijke sessies.
- OpenShift mount `maceclubheemskerk-sessions-local` op `/data`. De PVC vraagt met `volumeType: local` een Kubernetes local volume aan bij de bestaande local-path-provisioner, zodat OpenShift SELinux-labels en groepsrechten kan instellen. Deze PVC blijft bij deployments bewaard en wordt niet automatisch door Argo CD gepruned. Maak bij een clusterherstel ook deze data terug beschikbaar om sessies te behouden.
- Zolang bestandopslag wordt gebruikt: één replica en `Recreate`, zodat twee pods nooit onafhankelijk dezelfde sessies aanpassen. Dit geeft een korte onderbreking tijdens deploys. Meerdere replica's vragen later om gedeelde transactionele opslag zoals SQL.
- Lokaal gebruikt Docker Compose de named volume `sessions`. `docker compose down` bewaart de sessies; `down -v` verwijdert ze.
- Nieuwe beschermde API's moeten `Auth.RequireUser` gebruiken. Muterende requests worden daarnaast op de exacte Origin gecontroleerd. Er is geen auth-bypass voor productie of automatische testers.

## Nieuwe versies direct zichtbaar

Alle HTML, CSS, JavaScript en API-responses gebruiken `Cache-Control: no-store` en expliciete no-store-headers voor Cloudflare. De HTML verwijst naar CSS en JS met de versie in de URL. Er is geen service worker.

De fingerprint in `/api/version` is de SHA-256 van de daadwerkelijk draaiende Go-binary; ook backendwijzigingen tellen mee. Een zichtbare pagina controleert om de drie seconden en bij terugkeer naar de tab. Bij een verschil wordt de pagina automatisch vervangen door de nieuwe versie met een cache-busting queryparameter. Tijdelijke netwerkfouten laten de bestaande pagina intact. Tijdens de login-uitwisseling en zolang een dialoog (zoals het toevoegformulier of de videospeler) openstaat, wordt niet herladen. Een contentrevisie vernieuwt alleen de bibliotheek, zonder de pagina te herladen.

De sessiecookie en de PVC blijven bestaan bij herladen en deployen. Een tab die nog de oude versie van vóór deze monitor bevat, moet één keer handmatig worden vernieuwd om de monitor te laden. Het toevoegformulier houdt invoer vast bij validatiefouten en onderbrekingen; na een geslaagde opslag vernieuwt de bibliotheek direct.

## Losse pagina’s, media en bingo

- `/`: homepage; `/oefeningen`: oefenvideo’s (YouTube en eigen uploads), zonder categorieën; `/fotos-en-filmpjes`: het clubalbum met trainingen; `/wie-zijn-wij`: leden; `/bingo`: Lennarts excuses-bingo. Alle pagina’s zijn openbaar.
- Alleen leden uit `MEMBER_EMAILS` zien beheerknoppen. Titel en beschrijving zijn optioneel bij alle uploads. Beide lijsten hebben geen categorieën. Volgorde en prullenbak gelden per pagina. Oefeningen accepteren alleen video’s; foto’s horen in het clubalbum. Via Tekst → Pagina kun je een filmpje tussen de twee pagina’s verplaatsen.
- `PATCH /api/media/{id}` bewerkt tekst en optioneel `section`; `POST .../move` verplaatst omhoog/omlaag; `DELETE` verplaatst naar de prullenbak; `POST .../restore` herstelt. Mutaties vereisen de actuele `revision`; conflicten geven 409. Bestanden blijven bewaard, maar verwijderde media zijn niet openbaar opvraagbaar. `GET /api/media/trash` is alleen voor leden.
- `GET /api/bingo` is openbaar; `PATCH /api/bingo/{id}` is voor leden en bewaart `text`, `checked` en de verwachte `version`. De 25 startvakjes worden één keer ingevoegd; latere deploys bewaren de aanpassingen.

### Uploads en opslag

- `GET /api/media`: openbare bibliotheek en revisie; Google-subjecten en e-mailadressen van auteurs worden niet teruggegeven.
- `POST /api/media`: alleen een geldige sessie, toegestane Origin én een account in `MEMBER_EMAILS`.
- Video: JSON met `section` (`exercise`/`training`), `title`, `description`, `category` (oud veld, wordt genegeerd) en `url`. Alleen herkende YouTube-links worden opgeslagen als video-ID; de server haalt geen willekeurige URL op.
- Eigen video: `POST /api/videos`, multipart met `section`, `title`, `description`, `category` en bestand `video`. Dezelfde sessie-, Origin- en ledencontrole. Dit oudere endpoint accepteert nog maximaal 90.000.000 bytes, MP4/H.264. De browser gebruikt het nieuwe uploadprotocol hieronder. De Go-backend inspecteert begrensde MP4-metadata en streamt naar een tijdelijk bestand; pas na validatie verschijnt het in de bibliotheek. De browser toont voortgang en kan annuleren. Bestanden zijn openbaar via `/videos/{willekeurige-naam}.mp4`, met ondersteuning voor byte ranges en doorspoelen.
- Grote telefoonvideo’s: `POST /api/video-uploads` met titel, sectie, bestandsnaam, grootte en willekeurige `Upload-ID` start een upload. `PUT /api/video-uploads/{id}` verstuurt delen van maximaal 8 MiB met `Upload-Offset`. Een herhaald deel wordt op inhoud vergeleken; dubbele bevestigingen voegen geen bytes of records toe. `POST .../complete` start verwerking; `GET .../{id}` geeft de status; `DELETE .../{id}` annuleert. Elk endpoint controleert de sessie en clubrechten, wijzigingen controleren daarnaast Origin; uploads zijn aan de maker gebonden.
- Maximaal 1 GB invoer, MP4 of MOV, maximaal 30 minuten en 4096 pixels per as. FFmpeg zet de video automatisch om naar H.264/AAC (maximaal 1080p/30 fps); HEVC en HDR-telefoonvideo’s worden ondersteund. HDR wordt naar SDR omgezet. De originele upload en camerametadata worden na verwerking verwijderd; alleen de webversie wordt gepubliceerd en geback-upt. De Go-backend roept FFmpeg zonder shell aan; MOV-demuxer, lokale protocollen, geen externe datareferenties, begrensde threads, uitvoergrootte en verwerkingstijd.
- Eén video tegelijk uploaden/verwerken. De browser probeert kort onderbroken verzoeken opnieuw, bewaart de bestandskeuze bij fouten en toont upload- en verwerkingsstatus. Houd de pagina tijdens het uploaden open. Een procesherstart vereist opnieuw uploaden; inactieve uploads verlopen na 15 minuten. Conversie duurt maximaal 30 minuten en blijft na het sluiten van de pagina doorlopen. Er is ruimte nodig voor invoer plus maximaal 1 GB uitvoer; dat wordt vooraf gecontroleerd. De pod heeft maximaal twee CPU-cores en 1 GiB geheugen.
- Foto: multipart met `section=training`, `title`, `description`, bestand `photo`. Maximaal 10 MB en 16 megapixels; JPG, PNG of WebP. De backend controleert de echte inhoud, corrigeert JPEG-cameraoriëntatie, verkleint tot maximaal 2048 pixels en schrijft een nieuwe JPEG zonder originele metadata.
- YouTube-thumbnails komen van YouTube. De speler wordt pas na aanklikken via `youtube-nocookie.com` geladen. Geüploade foto’s zijn openbaar zichtbaar, zoals de rest van de homepage.
- Productie gebruikt PostgreSQL voor media en bingo. Foto’s en sessies blijven op PVC `maceclubheemskerk-sessions-local`. Lokaal kan ook SQLite op dit volume staan (WAL en transacties). Eén replica en `Recreate` vanwege de bestanden. Er wordt geen voorbeeldmedia ingeladen. Er is een bovengrens van 10.000 items; uploads worden één voor één verwerkt om geheugenverbruik te begrenzen.
- Video’s staan op een afzonderlijke 10 GiB PVC `maceclubheemskerk-videos`, gemount op `/videos` via `VIDEO_DIR`. Lokaal gebruikt Compose een apart volume. Het appbudget is 9 GiB, met een extra controle op vrije schijfruimte. De `local-path` storageclass reserveert niet fysiek 10 GiB op de node; bewaak ook de nodecapaciteit. Eén gelijktijdige videoupload; de bestaande database, foto’s en sessies behouden hun volume.
- Schema 3 migreert transactioneel met behoud van bestaande inhoud, IDs, bestanden en revisie. Het voegt volgorde, prullenbak en bingo toe. Test SQLite én PostgreSQL met `sh tests/postgres.sh` (tijdelijke database, geen productiecredentials). Oudere binaries ondersteunen deze beheerfuncties niet; gebruik een schemacompatibele versie bij terugrollen.

### Consistente back-up en herstel (SQLite)

Onderstaande opdracht is uitsluitend voor SQLite. Productie gebruikt PostgreSQL en vereist een PostgreSQL-back-up (bijvoorbeeld `pg_dump`) plus de foto- en videobestanden op de PVC’s. Het SQLite-back-upcommando werkt niet op PostgreSQL.

De Go-binary heeft een back-upcommando. Het maakt via `VACUUM INTO` een consistente databasekopie plus alle daarin genoemde foto’s en video’s in één zipbestand. Het overschrijft geen bestaande back-up. Lokaal:

```sh
docker compose exec web /server backup /videos/club-backup.zip
docker compose cp web:/videos/club-backup.zip ./club-backup.zip
```

Alleen bij een SQLite-installatie kan hetzelfde commando via `oc exec` worden gestart. De Alpine-runtime bevat FFmpeg en `tar`; een zip kan via `oc cp` naar externe opslag worden gekopieerd. Back-ups zijn handmatig; er is geen externe back-upbestemming geconfigureerd. Een back-up op dezelfde PVC beschermt niet tegen verlies van de node. Zorg vóór het back-uppen voor genoeg extra ruimte voor de hele bibliotheek; kopieer de zip daarna naar externe opslag.

Herstellen: stop de applicatie, bewaar de huidige `/data` en `/videos` apart, pak de zip uit naar een lege datamap (database en `uploads/` samen), zet de uitgepakte `videos/`-inhoud op de aparte `VIDEO_DIR`-mount, herstel passende eigenaar/rechten en start de app. Leg geen oude `-wal`/`-shm` naast een herstelde database. Sessies worden niet in de contentback-up opgenomen; behoud het bestaande `sessions.json` apart voor blijvende logins. Kopieer nooit alleen een actief SQLite-hoofdbestand voor een back-up.

Tests controleren SQLite-heropening, backup/herstel met foto’s, Origin en ledenrechten, uploadvalidatie, normalisatie en bescherming tegen toegang tot andere bestanden. Echte Google-productielogins worden niet automatisch getest.
