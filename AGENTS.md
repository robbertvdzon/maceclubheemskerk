# Projectafspraken

- De backend is volledig Go. Google ID-tokens worden geverifieerd met de officiële Google Go-library; overige logica gebruikt de standaardbibliotheek.
- Bouw en controleer via Docker Compose; een lokale Go-installatie is niet nodig.
- De website draait uiteindelijk op Robberts OpenShift-cluster achter de bestaande Cloudflare Tunnel.
- Content staat in SQLite via de pure-Go modernc-driver. Database en foto’s staan op de bestaande PVC; er is geen externe SQL-server nodig. Versieer toekomstige schemamigraties expliciet.
- Houd HTTP-afhandeling in `internal/web` en procesconfiguratie in `cmd/server`.
- HTML en CSS worden in de Go-binary opgenomen met `go:embed`.
- Voeg geen verzonnen trainingstijden, contactgegevens of clubinformatie toe.
- Controleer wijzigingen met `docker compose run --rm tools go vet ./...` en een containerbuild. Gebruik gerichte tests bij nieuwe logica.
- Deployment: GitHub Actions bouwt linux/amd64 naar GHCR en pint de image-digest in `deploy/kustomization.yaml`; Argo CD rolt uit naar namespace `maceclubheemskerk`. De Application staat in `robberts-infrastructure`.
- `secrets.env` blijft lokaal en buiten Docker-builds. Lees env-bestanden als data, nooit met `source` of `eval`. Alleen versleutelde SealedSecrets mogen in Git.
- Bij deployment- of secretswijzigingen: `python3 -m unittest discover -s tests -v`, `oc kustomize deploy` en `python3 deploy/smoke-test.py <image>`.
- Google-login geeft een eigen intrekbare HttpOnly-sessie, 365 dagen geldig en bij gebruik verlengd. Sessies worden als tokenhashes op een PVC bewaard; gebruik één replica met Recreate zolang deze bestandopslag wordt gebruikt. Later kan de opslaglaag naar SQL.
- Alle account-API's moeten server-side RequireUser gebruiken; authenticatie alleen geeft geen beheerdersrechten. Geen productie-testlogin of auth-bypass toevoegen.
- HTML, assets en API's mogen niet gecachet worden. Houd de versiecontrole en automatisch herladen met behoud van de sessie in stand.

- Alleen accounts uit `MEMBER_EMAILS` mogen content toevoegen. Een lege lijst weigert alle schrijfacties; login alleen is nooit voldoende. Geen openbare registratie of aanmeldformulier.
- SQLite staat standaard in `/data/maceclub.sqlite`, foto’s in `/data/uploads`. Bewaar `/data/sessions.json` bij wijzigingen zodat bestaande logins geldig blijven.
- Het goedgekeurde ontwerp staat in `design/`; de echte website staat in `internal/web/static`. Voorbeeldcontent en gesimuleerde login mogen niet naar de live frontend worden gekopieerd.
- Uploads zijn begrensd, worden als afbeelding gedecodeerd en opnieuw gecodeerd. Bewaar willekeurige bestandsnamen. Tijdens een open formulier of videospeler geen automatische paginareload.
