# Mace Club Heemskerk

Go-webserver met een tijdelijke Nederlandse startpagina. Geen database, JavaScript-build of lokale Go-installatie nodig. Docker compileert de Go-code; de runtime-image bevat alleen de binary en CA-certificaten.

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

Er zijn nog geen Go-unit-tests; de webserver wordt met een build en HTTP-smoketests gecontroleerd. Voor de secrets-tooling zijn er gerichte Python-tests:

```sh
python3 -m unittest discover -s tests -v
docker build --platform linux/amd64 -t maceclubheemskerk:verify .
python3 deploy/smoke-test.py maceclubheemskerk:verify
oc kustomize deploy
```

De smoketest gebruikt een eigen tijdelijke container met een willekeurige UID, alleen-lezen bestandssysteem en zonder Linux-capabilities. Hij controleert HTML, CSS, healthcheck, foutcodes en netjes stoppen. Je lokale website blijft draaien.

## Structuur

- `cmd/server`: opstarten, timeouts en netjes stoppen bij SIGTERM.
- `internal/web`: HTTP-routes en ingebouwde HTML/CSS.
- `deploy`: OpenShift Deployment, Service, Routes, image-pin en SealedSecrets-tooling.
- `.github/workflows/deploy.yml`: verificatie, image publiceren en GitOps image-pin.
- `GET /healthz`: status voor OpenShift-probes.
- `PORT`: luisterpoort, standaard `8080`.

Later kan SQL vanuit Go via `database/sql` en een driver worden aangesloten. Voeg de opslaglaag toe wanneer de functionaliteit en databasekeuze bekend zijn. Er is nu geen opslag of schijnpersistentie.

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

### Secrets voor de latere SQL-koppeling

`secrets.example.env` is het lege voorbeeld in Git. De lokale `secrets.env` is aangemaakt met bestandsrechten `600` en wordt door Git én Docker genegeerd. Maak bij een nieuwe checkout zelf een kopie:

```sh
cp -n secrets.example.env secrets.env
chmod 600 secrets.env
```

De gereserveerde keys zijn `DATABASE_URL`, `DATABASE_USER` en `DATABASE_PASSWORD`. De databasekeuze en het URL-formaat volgen later. De Go-app gebruikt deze waarden nu nog niet; er wordt geen database of databasegebruiker aangemaakt. Lege waarden mogen blijven staan.

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
- De lokale Compose-service leest `secrets.env` voorlopig niet; dit wordt aangesloten wanneer de Go-app daadwerkelijk databaseconfiguratie gebruikt.

### Terugrollen

Herstel de vorige werkende image-digest in `deploy/kustomization.yaml` en commit die met `[skip ci]`. Argo CD synchroniseert terug naar die versie. Zo overschrijft een nieuwe imagebuild de rollback niet direct.
