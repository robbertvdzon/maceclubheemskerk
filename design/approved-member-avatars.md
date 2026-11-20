# Goedgekeurde clubavatars

De website gebruikt de door Robbert goedgekeurde voorstellen uit deze taak. De
illustraties zijn gemaakt met de ingebouwde imagegen-tool. De WebP-bestanden zijn
verliesvrije omzettingen van de volledige originele PNG's; de website toont alleen
het betreffende portret via een SVG-uitsnede. Namen en ondertitels blijven HTML-tekst.
Er is voor deze plaatsing niets opnieuw getekend.

| Lid | Bestand | Goedgekeurd voorstel |
| --- | --- | --- |
| Rotsvaste Robbert | `internal/web/static/robbert-approved.webp` | `exec-1000ece9-e26c-4f63-ab25-53b3d5433cfa.png`: gladde, langere mace over de schouder; ader verwijderd |
| Strong Steve | `internal/web/static/steve-approved.webp` | `exec-a07f8383-8fed-4200-a09e-88f92bc0e087.png`: één zware mace rechtop en twee verschillende gewichten achter de nek |
| Muscle Martijn | `internal/web/static/club-approved.webp`, linksonder | `exec-45fa6d9f-20ef-4d59-916f-5607ae86310a.png`: nieuwe foto als gelijkenis, captain-band en espresso |
| Lazy Lennart | `internal/web/static/club-approved.webp`, rechtsonder | Hetzelfde goedgekeurde overzicht: bingokaart en ananas |

De uitsneden staan in `internal/web/static/index.html`. Het clipPath voorkomt dat
andere portretten of de in de afbeelding opgenomen teksten zichtbaar worden.
Lennart gebruikt dezelfde goedgekeurde illustratie op de bingopagina.

## Sexy Steve — 20 november 2026

Steve gebruikt nu `internal/web/static/sexy-steve.webp`, een verliesvrije WebP van
`design/sexy-steve/sexy-steve-v1.png`. De uitsnede toont de hele figuur met de korte
glitterbroek; de postertekst is vervangen door de HTML-titel “Sexy Steve”.

De oude afbeelding blijft ongewijzigd in `internal/web/static/steve-approved.webp`
en blijft bereikbaar via `/steve-approved.webp`. Terugzetten: verander bij Steve
in `index.html` de afbeelding naar `/steve-approved.webp?v={{.Version}}`, zet
viewBox en clipPath terug naar `0 90 1024 1125` (rect x=0, y=90, width=1024,
height=1125), herstel SVG width=1024 en height=1125, de titel “Smelly Steve” en
de alttekst “Smelly Steve met één zware mace en twee maces over zijn schouders”.
