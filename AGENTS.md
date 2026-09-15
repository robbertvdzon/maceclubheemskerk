# Projectafspraken

- De backend is volledig Go. Gebruik voorlopig de standaardbibliotheek.
- Bouw en controleer via Docker Compose; een lokale Go-installatie is niet nodig.
- De website draait uiteindelijk op Robberts OpenShift-cluster achter de bestaande Cloudflare Tunnel.
- Er is nu geen database. Voeg SQL, een driver of migraties pas toe wanneer Robbert de gegevensfunctionaliteit vraagt.
- Houd HTTP-afhandeling in `internal/web` en procesconfiguratie in `cmd/server`.
- HTML en CSS worden in de Go-binary opgenomen met `go:embed`.
- Voeg geen verzonnen trainingstijden, contactgegevens of clubinformatie toe.
- Controleer wijzigingen met `docker compose run --rm tools go vet ./...` en een containerbuild. Gebruik gerichte tests bij nieuwe logica.
- Deployment: GitHub Actions bouwt linux/amd64 naar GHCR en pint de image-digest in `deploy/kustomization.yaml`; Argo CD rolt uit naar namespace `maceclubheemskerk`. De Application staat in `robberts-infrastructure`.
- `secrets.env` blijft lokaal en buiten Docker-builds. Lees env-bestanden als data, nooit met `source` of `eval`. Alleen versleutelde SealedSecrets mogen in Git.
- Bij deployment- of secretswijzigingen: `python3 -m unittest discover -s tests -v`, `oc kustomize deploy` en `python3 deploy/smoke-test.py <image>`.
