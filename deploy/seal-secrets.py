#!/usr/bin/env python3
"""Convert local env data to a namespace-bound SealedSecret, without a cluster write."""

import argparse
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[1]
ALLOWED_KEYS = {"DATABASE_URL", "DATABASE_USER", "DATABASE_PASSWORD"}


def read_values(path):
    values = {}
    seen = set()
    for number, raw in enumerate(path.read_text().splitlines(), 1):
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        key, separator, value = line.partition("=")
        key = key.strip()
        if not separator or key not in ALLOWED_KEYS:
            raise ValueError(f"Ongeldige of niet-toegestane key op regel {number}.")
        if key in seen:
            raise ValueError(f"Dubbele key op regel {number}.")
        seen.add(key)
        value = value.strip()
        if value.startswith(('"', "'")):
            if len(value) < 2 or value[-1] != value[0]:
                raise ValueError(f"Onvolledige quotes op regel {number}.")
            value = value[1:-1]
        if value:
            values[key] = value
    return values


def atomic_write(path, contents):
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, temporary = tempfile.mkstemp(dir=path.parent)
    try:
        with os.fdopen(fd, "w") as target:
            target.write(contents)
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def seal(source, cert, destination):
    values = read_values(source)
    if not values:
        raise ValueError("Geen ingevulde waarden; bestaand SealedSecret blijft behouden. De site kan zonder secrets starten.")
    metadata = {"name": "maceclubheemskerk-secrets", "namespace": "maceclubheemskerk"}
    secret = {"apiVersion": "v1", "kind": "Secret", "metadata": metadata,
              "type": "Opaque", "stringData": values}
    result = subprocess.run(
        ["kubeseal", "--cert", str(cert), "--scope", "strict", "--format", "json"],
        input=json.dumps(secret), text=True, capture_output=True, check=False,
    )
    if result.returncode:
        # Tool errors might repeat input. Never print raw stderr or plaintext.
        raise ValueError("Versleutelen mislukt; controleer kubeseal en het clustercertificaat. Bestaande bestanden blijven behouden.")
    sealed = json.loads(result.stdout)
    if (sealed.get("kind") != "SealedSecret"
            or set(sealed.get("spec", {}).get("encryptedData", {})) != set(values)):
        raise ValueError("Onverwachte kubeseal-uitvoer; niets opgeslagen.")
    # Only persist the encrypted fields and known metadata, never arbitrary tool output.
    clean = {"apiVersion": "bitnami.com/v1alpha1", "kind": "SealedSecret",
             "metadata": metadata,
             "spec": {"encryptedData": sealed["spec"]["encryptedData"],
                      "template": {"metadata": metadata, "type": "Opaque"}}}
    atomic_write(destination / "sealed-secret.json", json.dumps(clean, indent=2) + "\n")
    atomic_write(destination / "kustomization.yaml",
                 "apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\nresources:\n  - sealed-secret.json\n")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", type=Path, default=ROOT / "secrets.env")
    parser.add_argument("--cert", type=Path, default=ROOT.parent / "robberts-infrastructure/manifests/cluster-bootstrap/cluster-cert.pem")
    args = parser.parse_args()
    try:
        seal(args.source, args.cert, ROOT / "deploy/secrets")
    except (OSError, ValueError):
        print("Sealing mislukt: controleer bronbestand, toegestane keys, ingevulde waarden en clustercertificaat. Er worden geen secretwaarden getoond.", file=sys.stderr)
        return 1
    print("SealedSecret bijgewerkt voor namespace maceclubheemskerk. Commit alleen deploy/secrets, nooit secrets.env.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
