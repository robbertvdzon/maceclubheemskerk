# UX-ontwerp — Mace Club Heemskerk

Klikbaar, responsive ontwerp; nog geen wijzigingen aan de live website, Google-login of opslag.
Start met `python3 -m http.server 18081 --directory design` vanuit de projectmap.

## Ontwerprichting

- Het originele aangeleverde logo, zwart/olijf, gebroken wit en warm oranje.
- Publiek: oefeningen met YouTube-video’s, eigen trainingsvideo’s en foto’s, de clubleden.
- Mobiel: zichtbare sectienavigatie, oefeningen onder elkaar, compact fotoalbum, grote knoppen.
- Profielen gebruiken initialen totdat echte portretten beschikbaar zijn. Namen: Robbert, Stephaan, Martijn, Lennart. De club kan later groeien; de teksten gaan niet uit van een vast aantal leden.
- Het browsericoon is een eenvoudige nieuwe vector met de gekruiste macebells, leesbaar op tabformaat.
- De onderste ontwerpbalk wisselt tussen bezoeker en clublid; ‘Voorbeelden’ uit toont lege secties.
- De login, mediaviewer en toevoegformulieren zijn klikbare voorbeelden. Nieuwe kaarten bestaan alleen in het geheugen van deze pagina. Eigen fotovoorbeelden verlaten de browser niet.
- Er zijn geen verzonnen YouTube-links, portretten of trainingsfoto’s gebruikt. De illustraties en titels zijn expliciet ontwerpvoorbeelden.

## Werking na goedkeuring

1. De homepage blijft openbaar. Bestaande Google-login en blijvende sessies blijven behouden.
2. Alleen expliciet toegestane Google-accounts van clubleden mogen content toevoegen. De lijst kan later worden uitgebreid. Er komt geen openbare registratie, aanmeldformulier of wervende oproep; nieuwe leden komen via persoonlijke contacten. De server controleert dat bij iedere schrijfactie; alleen een verborgen knop is onvoldoende. Hiervoor zijn later de vier accountadressen nodig.
3. SQLite komt in `/data/maceclub.sqlite`, op de bestaande werkende PVC `maceclubheemskerk-sessions-local`. Eén Go-proces/replica blijft schrijven. Gebruik transacties, een busy timeout en WAL. De SQLite-driver moet geschikt zijn voor de bestaande Docker-build.
4. Tabel `media`: id, section (exercise/training), kind (youtube/photo), title, description, category, youtube_id of photo_path, created_by_google_sub, created_at. Server valideert de combinatie van velden.
5. Foto’s in `/data/uploads/`, met willekeurige bestandsnamen; SQLite bewaart de metadata en verwijzing. Bestandsinhoud, type, grootte en afbeeldingsafmetingen worden op de server gecontroleerd. YouTube-video’s blijven bij YouTube; alleen de video-ID wordt opgeslagen.
6. De bestaande sessieopslag blijft tijdens deze uitbreiding intact. Sessies kunnen later via een aparte migratie naar SQLite; bestaande logins mogen niet ongemerkt vervallen.
7. Back-up: SQLite online backup API of een consistente backup, plus geüploade foto’s. Alleen het .sqlite-bestand kopiëren terwijl WAL actief is, is geen betrouwbare backup. Een PVC beschermt tegen podherstarts, niet tegen verlies van de hostschijf.
8. Contentwijzigingen worden zichtbaar zonder een nieuwe deployment. De versiecontrole krijgt een contentversie. Tijdens een open formulier wordt invoer behouden en automatisch herladen uitgesteld; na opslaan wordt de lijst direct vernieuwd.

## Nog te bouwen na ontwerpbeoordeling

Go/SQLite-opslag, echte YouTube-embeds, foto-upload API, autorisatie voor toegestane clubaccounts, backups en koppeling met de homepage. Geen van deze functies is in dit ontwerp al als productiefunctionaliteit geïmplementeerd.
