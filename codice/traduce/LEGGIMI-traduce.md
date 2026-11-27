# Tasso Traduce

Traduce i giochi **sul PC**: le scritte dello schermo, le voci dei personaggi (sottotitoli) e la
chat. Tutte le traduzioni si fanno sul PC, nessun testo viene mandato fuori. Strumento a sé
della famiglia Tasso: non modifica né il programma Tasso né il sito. Windows 10/11 64 bit, un
solo file `TassoTraduce.exe`, niente installazione, niente permessi da amministratore.

Questa prima parte descrive la versione attuale (0.16). Più sotto c'è la storia delle versioni:
lì alcuni numeri e motori sono **superati** (per esempio il vecchio correttore Qwen3-8B).

## Cosa fa (versione 0.16)

- **Traduzione dello schermo** (F7 nel gioco, o da sola con "Traduzione gioco"): legge tutte le
  scritte della finestra del gioco con la lettura del testo di Windows, le traduce e le rimette
  al loro posto, con colori e grandezza simili. Funziona con qualsiasi gioco in finestra o
  "schermo intero senza bordi", anche con l'anti-imbroglio (non tocca il gioco).
- **Traduzione gioco** (a passi): scegli il gioco, la sua lingua, la lingua che vuoi e premi
  Applica. Poi "Apri il gioco" e Tasso controlla davvero se la traduzione parte: dice
  "Funziona" solo quando l'ha visto.
- **Dentro il gioco** (facoltativo, solo giochi Unity "Mono" o IL2CPP fino a Unity 6.5, senza
  anti-imbroglio, da inglese a italiano): aggiunge BepInEx + XUnity AutoTranslator nella cartella
  del gioco. I file del gioco non vengono cambiati; i file aggiunti sono elencati e "Ferma
  traduzione" li toglie. Unity 6.6 (es. Moon Sweepers) non è supportato dagli strumenti liberi:
  quei giochi si traducono sullo schermo.
- **Lingue**: da e verso inglese, italiano, francese, tedesco, spagnolo, portoghese, polacco,
  russo, giapponese, coreano, cinese semplificato e tradizionale, danese, finlandese, norvegese,
  svedese, turco. Fra due lingue senza modello diretto si passa dall'inglese. Ogni lingua si
  scarica solo quando la scegli (22–70 MB per direzione). Per russo, giapponese, coreano e cinese
  Windows deve saper leggere quella lingua: Tasso apre le Impostazioni di Windows dove la aggiungi.
- **Voci** (Ctrl+Alt+V): sottotitoli di quello che dicono i personaggi (solo voci in inglese).
- **Chat** (Ctrl+Alt+T): scrivi in italiano, esce in inglese.
- **Correttore IA** facoltativo (EuroLLM-9B, sul PC): ricontrolla le traduzioni in sottofondo e le
  migliora; si fa da parte se il gioco ha bisogno della scheda video o del processore.

## Qualità e velocità misurate (onestamente)

- Inglese → italiano, 1000 frasi di giochi scritte apposta (prova\test_1000.jsonl), chrF++:
  Mozilla 70,3; con il correttore EuroLLM 73,0 (tutte le 1000). Sulla metà mai usata per
  scegliere: 70,18 → 72,98 (Qwen3-8B 71,64): numero ottenuto il 22/11 ma il file del risultato
  non è stato salvato. Limiti: 9 frasi della prova sono anche nel catalogo delle frasi fisse
  ("Press any key to continue.", "You died."…), alcune regole dei termini sono nate guardando
  errori di queste stesse frasi, e le frasi di riferimento potrebbero favorire un correttore IA.
- Altre lingue: frasi FLORES-200 (testi di Wikipedia, NON di giochi), 200 frasi per coppia,
  solo Mozilla (collaudo\qualita-lingue): inglese→italiano 57,9; tedesco 54,5; portoghese 54,9;
  francese 54,0; danese 53,6; svedese 52,4; norvegese 50,7; spagnolo 50,6; polacco 50,2;
  russo 49,8; finlandese 49,1; cinese semplificato 48,7; turco 48,3; giapponese 47,3; cinese
  tradizionale 45,8; coreano 45,4 (tutte verso l'italiano). Altre coppie: portoghese→francese
  61,5; inglese→francese 68,1; inglese→tedesco 64,4; tedesco→francese 59,9.
  **Le più deboli: coreano, cinese tradizionale, giapponese, turco.** Nessuna coppia diversa da
  inglese→italiano è stata provata su testi di giochi veri.
- Correttore IA per le altre lingue (50 frasi per coppia, FLORES, metà di sviluppo): migliora
  tutte le 10 coppie provate di 0,6–2,1 punti (es. coreano→italiano 45,9 → 47,6, inglese→tedesco
  64,6 → 66,7), ma costa circa 1 s a frase sulla scheda video. Il numero sulla metà di verifica
  è nel paragrafo della versione 0.16.
- Velocità dello schermo (0.16): un cambio di schermata tradotto in 133–217 ms, misurato con 2
  immagini fisse di prova mostrate a tutto schermo (non un gioco vero), monitor acceso; 4 cambi
  su 6 avevano scritte già viste. La parte "colori e sfondi" del tempo non è precisa.
- Voci: 0,3–0,5 s dopo la fine della frase nelle prove migliori; con musica sotto le parole giuste
  sono 63–65 su 67 a seconda della prova. Non provato in un gioco vero.

## Privacy e sicurezza (cosa va su internet, e cosa no)

- I testi dei giochi, della chat e delle voci **non escono mai dal PC**.
- Si scaricano da internet solo i modelli e gli strumenti, dopo la tua scelta nel programma, da
  indirizzi fissi (Mozilla, Hugging Face, GitHub, build di BepInEx) e **ognuno controllato con la
  sua impronta SHA-256**.
- Eccezioni da sapere: (1) con "Dentro il gioco" su un gioco IL2CPP, alla prima partenza è
  **BepInEx** a scaricare le librerie di Unity da unity.bepinex.dev (manda solo il numero di
  versione di Unity; questo file Tasso non lo controlla); (2) WebView2 di Windows, che disegna la
  finestra, può collegarsi a Microsoft per i suoi aggiornamenti; (3) aggiungere la lettura di una
  lingua a Windows passa dalle Impostazioni di Windows.
- I server interni ascoltano solo su 127.0.0.1: la finestra con un percorso segreto casuale;
  il correttore IA con una chiave; il server per "Dentro il gioco" risponde solo ai giochi con la
  traduzione installata, con un segreto diverso per ogni gioco, e rifiuta le pagine web. Il
  server delle voci (whisper) non ha chiave: un altro programma del PC potrebbe usarlo (solo
  consumo di processore, nessun dato esce).
- Sul disco restano: la memoria delle traduzioni, le librerie dei giochi, e per "Dentro il gioco"
  l'elenco delle frasi chieste dal gioco (logs\traduzioni-gioco-*.txt). L'immagine della schermata
  tradotta la salva solo la versione di prova.
- Mai file aggiunti ai giochi con l'anti-imbroglio (EasyAntiCheat, BattlEye, Vanguard, ACE,
  GameGuard, XIGNCODE, FACEIT…): quelli si traducono sullo schermo. Niente disattiva o aggira
  l'anti-imbroglio, Defender o SmartScreen. Il programma non è firmato: SmartScreen può avvisare.

## Compilare

Serve Go 1.26 (https://go.dev/dl/), senza CGO:

    set CGO_ENABLED=0
    go test .
    go build -trimpath -ldflags "-H windowsgui -s -w" -o TassoTraduce.exe .

La versione di prova (foto, collaudi, prove delle lingue) si compila con `-tags collaudo`.
WebView2 è nella cartella `third_party\go-webview2` (con piccole correzioni nostre, segnate nel
codice). Le librerie di frasi sono in `dati\` (vedi `dati\libreria-giochi-liberi-LICENZA.txt`).

## Versione 0.16 (22/11/2026): tutte le lingue e controllo con 7 revisori

- Tutte le 16 lingue dei giochi, da qualsiasi lingua a qualsiasi altra (vedi sopra), con
  riconoscimento automatico (alfabeto latino) e lettura dello schermo nella lingua giusta.
- Librerie di frasi tradotte a mano anche per le altre lingue di arrivo (stesse fonti libere).
- Correttore IA anche per le altre lingue. Sulla metà di **verifica** (50 frasi FLORES per coppia,
  mai usate prima) migliora tutte le 10 coppie provate: tedesco→italiano 54,0→55,1;
  francese→italiano 54,0→55,5; spagnolo→italiano 51,4→51,9; giapponese→italiano 45,9→46,7;
  coreano→italiano 44,8→46,2; cinese→italiano 48,7→49,8; russo→italiano 49,4→50,8;
  portoghese→francese 61,1→62,8; inglese→tedesco 63,5→64,4; tedesco→francese 59,4→60,6.
  Circa 1 s a frase sulla scheda video (in sottofondo: si vede prima Mozilla). Per coreano e
  giapponese le frasi peggiorate non sono poche (12 e 9 su ~26 cambiate): il guadagno è piccolo.
- Controllo finale con 7 revisori scettici: circa 65 problemi trovati e corretti, tra cui:
  traduzione ferma quando cambiano solo le parole nello stesso riquadro; lettura dello schermo
  ripetuta all'infinito sui giochi già nella lingua giusta; gioco ridotto a icona che fermava la
  traduzione; server "Dentro il gioco" chiamabile da pagine web (ora segreto + controlli);
  anti-imbroglio non visto per giochi fuori dalle cartelle dei negozi; manifesto dei file
  aggiunti scritto troppo tardi; plugin dell'utente cancellati da Ferma (ora messi da parte);
  un errore nel correttore IA che lo fermava per sempre; memoria delle traduzioni non salvata
  all'uscita; percorso di WebView2 che creava cartelle con nomi a caso; chiusure silenziose (ora
  messaggio); se Tasso è già aperto ora viene portato davanti; aggiunta lingua OCR tramite le
  Impostazioni di Windows invece di un comando da amministratore.

# Storia delle versioni (alcuni dati superati)

## Versione 0.15 (22/11/2026): schermo nero corretto

- Sul PC di Sergio la cattura veloce di Windows (duplicazione del desktop) restituisce
  un'immagine tutta nera quando i monitor sono accesi; la cattura classica invece vede tutto.
  Ora, se la cattura veloce dà nero e quella classica no, uso la classica. Ignoro anche i
  fotogrammi "solo mouse" che non contengono l'immagine.
- Prova dal vivo con i monitor accesi: cambio schermata tradotto in **125–195 ms**
  (6 cambi, scritte giuste).
- Giochi Unity molto recenti (6.6, come Moon Sweepers): si traducono sullo schermo, e la
  procedura lo dice in una riga. La traduzione dentro il gioco per loro non c'è (gli strumenti
  liberi non li leggono ancora: BepInEx issue 1428, Il2CppInterop issue 295); scelta di Sergio:
  basta lo schermo.

## Versione 0.14 (22/11/2026): correzioni dopo la prova di Sergio su Moon Sweepers

- **Cosa era successo**: Moon Sweepers è fatto con Unity 6.6 (dati IL2CPP versione 108). Gli
  strumenti liberi (BepInEx) leggono fino alla 106: la traduzione dentro il gioco non è partita
  (scritto nel registro di BepInEx), ma Tasso aveva detto "applicata". In più il gioco era in
  tedesco, e Tasso traduce dall'inglese.
- Ora, prima di proporre "Dentro il gioco", controllo la versione dei dati: se è troppo nuova la
  scelta è spenta con il motivo, e si traduce sullo schermo.
- Se la traduzione dentro non parte (registro di BepInEx), passo subito allo schermo e, a gioco
  chiuso, tolgo da solo i file aggiunti: il gioco torna com'era. Su Moon Sweepers l'ho fatto
  il 22/11: la cartella del gioco è di nuovo identica all'originale.
- Passo 2: non dico più "inglese riconosciuto". Chiedo in che lingua hai messo il gioco; se non
  è inglese ti dico di metterlo in inglese (o in italiano, se il gioco ce l'ha).
- Sullo schermo riconosco la lingua delle scritte: se sono in tedesco, francese, spagnolo o già
  in italiano lo dico e non disegno traduzioni sbagliate.
- Dopo Applica: niente più "Traduzione applicata". C'è **Apri il gioco** (da Steam o Epic) e poi
  controllo davvero: "Funziona: N scritte tradotte" solo quando l'ho visto.
- Non ancora provato con un gioco aperto: durante la prova lo schermo del PC era nero (bloccato).

## Versione 0.13 (22/11/2026): impostazioni semplici

- "Analizza il mio PC" dice come va in una frase; scheda video, memoria e processore sono dietro
  "Mostra dettagli".
- Al posto del riquadro "Motore di traduzione" c'è un solo interruttore: **Traduzione più
  precisa (IA)**, acceso di base (spento = solo il traduttore veloce).
- Tasti: in vista solo F6 "Traduci lo schermo"; gli altri tasti e le parole fisse sotto "Avanzate".

## Versione 0.12 (22/11/2026): più veloce

- **Sullo schermo**: quando la schermata cambia, lettura e traduzione partono subito, mentre
  controllo che la schermata si sia fermata (prima aspettavo e poi leggevo). Se intanto si è
  mossa, rifaccio sull'immagine nuova. Il salvataggio della memoria va in sottofondo e il
  controllo dello schermo è più frequente (ogni 40 ms invece di 60).
  Misura dal vivo sulle stesse 2 schermate (6 cambi): **prima 430–645 ms (media 542), ora
  379–400 ms (media 383)**, stesse scritte e stesse traduzioni. Su questo PC metà del tempo è
  la cattura dello schermo (~235 ms, monitor in risparmio energetico): con il monitor acceso
  Windows usa la cattura veloce e il tempo scende ancora, ma questo non l'ho potuto misurare.
- **Dentro il gioco**: XUnity aspettava 1 secondo prima di chiedere ogni scritta nuova; ora
  0,1 s, e niente limiti "antispam" (servono per i siti in rete, non per Tasso sul PC).
  Quindi ogni scritta nuova arriva circa 0,9 s prima. Non misurato in un gioco vero.

## Versione 0.11 (22/11/2026): libreria grande e anti-imbroglio

- **Libreria di 27.117 frasi tradotte a mano** dai traduttori volontari di 5 giochi liberi
  (Battle for Wesnoth, SuperTuxKart, 0 A.D., Luanti, Freeciv; licenze GPL/LGPL compatibili, vedi
  `dati/libreria-giochi-liberi-LICENZA.txt`). Se una frase del gioco è identica (4 parole o più),
  esce subito la traduzione umana: 0,0007 ms invece di 20 ms di Mozilla o 0,4 s dell'IA. Le
  scritte corte restano al catalogo e al traduttore, perché nella libreria cambiano senso da gioco
  a gioco ("Got it" → "Trovata!"): prova fatta sulle 446 frasi del catalogo, metà diverse.
  La libreria si carica in sottofondo all'avvio (0,13 s).
- Provato anche a usare la libreria come esempi per il correttore IA (3 o 2 frasi simili nel
  messaggio): sulla metà di sviluppo delle 1000 frasi **peggiora** (72,96 → 72,66 / 72,58) ed è
  più lento (0,36 → 0,55 s). Quindi NON è attivo. Il numero sulla metà di verifica resta 72,98.
- Le 1000 frasi di prova sono state tolte dalla libreria (16 c'erano), così la misura è onesta;
  sulle 1000 frasi la libreria quindi non cambia il numero: aiuta nei giochi veri quando le
  frasi coincidono.
- **Giochi con anti-imbroglio**: ora si traducono anche loro, in automatico, **sullo schermo**
  (mai file aggiunti). Esce solo un piccolo messaggio: "Per non rischiare con l'anti-imbroglio,
  questo gioco viene tradotto sullo schermo".
- Traduzione dentro il gioco: se dopo 4 minuti di gioco non arriva nessuna scritta, passo da solo
  alla traduzione dello schermo.

## Versione 0.10 (22/11/2026): traduzione dentro il gioco (giochi Unity)

- Nella **Traduzione gioco**, al passo 3, per i giochi **Unity** c'è la scelta **Dentro il gioco**:
  il testo viene preso direttamente dal gioco e rimesso tradotto con il suo carattere (niente
  lettura dello schermo, quindi niente errori di lettura). Gli altri giochi restano "Sullo schermo".
- Come: aggiungo accanto al gioco due strumenti liberi, **BepInEx** (LGPL-2.1) e **XUnity
  AutoTranslator** (MIT), scaricati dalle pagine ufficiali con impronta di sicurezza. Di XUnity
  installo SOLO il pezzo che chiede le traduzioni a Tasso sul PC (indirizzo 127.0.0.1): i
  traduttori in rete (Google, DeepL…) non vengono installati, nessun testo esce dal PC.
- Sicurezza: mai con l'anti-imbroglio; mai se il gioco ha già delle mod; nessun file del gioco
  viene cambiato (solo aggiunti; se uno esistesse già ne faccio prima la copia). Un elenco di
  tutto quello che aggiungo permette a **Ferma traduzione** di rimettere il gioco esattamente
  com'era (provato su una cartella di prova: identica prima e dopo). Le traduzioni fatte restano
  da parte per la volta dopo.
- Tasso Traduce deve essere aperto mentre giochi. Le frasi migliorate dal correttore IA si
  vedono dalla volta dopo che apri il gioco. Se Steam aggiorna il gioco, rifai Applica.
- Giochi Unity "IL2CPP" (come Moon Sweepers e Liar's Bar): la prima partenza dopo Applica dura
  qualche minuto in più (BepInEx prepara i suoi file una volta sola).
- **Non ancora visto funzionare dentro un gioco vero**: la prova sul tuo PC va fatta da te
  (vedi sotto). Non usarlo nelle partite online classificate.

## Versione 0.9 (22/11/2026): Traduzione gioco a passi

- Nuovo strumento **Traduzione gioco** (anche cliccando la copertina in "I miei giochi"):
  1) scegli il gioco, 2) lingua del gioco (inglese), 3) lingua che vuoi (italiano), poi
  **Applica traduzione**. La barra di avanzamento mostra i passi veri: controllo del traduttore,
  libreria delle traduzioni del gioco, prova su scritte tipiche, controllo del correttore IA.
- Da lì in poi quel gioco **si traduce da solo appena lo apri**, senza premere F6
  (gli altri giochi restano come prima). **Ferma traduzione** lo toglie.
- Differenza voluta da altri programmi: **non cambio nessun file del gioco**. "Applicare" vuol
  dire solo ricordare il gioco e tradurre lo schermo con la sovrimpressione. I giochi con
  anti-imbroglio non partono mai da soli: lì solo F6, a tuo rischio.
- Onestà: la qualità e la velocità della traduzione sono quelle della 0.7/0.8 (Mozilla 70, con
  EuroLLM 73 sulle 1000 frasi; cambio schermata 0,4-0,7 s; voci 0,3-0,5 s). "Due volte più
  potente" non si può misurare così: questa versione rende più semplice e automatico l'uso.
- Provato sul PC: procedura completa su un gioco vero della libreria Steam, avanzamento,
  successo e Ferma (le impostazioni tornano com'erano). Non ancora provato dentro un gioco aperto.

## Versione 0.8 (22/11/2026): finestra nuova

Finestra rifatta in stile "app per giocatori" (ispirata a LagoFast, senza copiarne marchi o grafica):
barra laterale con le icone e i tuoi giochi con la copertina; pagina "Strumenti di gioco" con il
pulsante grande per accendere e il pannello degli strumenti a schede (Tutti, Traduzione, Voce, Chat);
ogni strumento si apre in una finestrella con lingua del gioco → lingua di arrivo, scorciatoia con
"Cambia", interruttore e "Salva e applica"; pagina "I miei giochi" con le copertine. Le funzioni di
LagoFast che toccano i giochi (aumento FPS, DLSS iniettato, cambio IP) restano fuori per le regole
anti-cheat. Traduzione e velocità uguali alla 0.7.

## Versione 0.7 (22/11/2026): traduzione migliore

| 1000 frasi, dal vivo sul PC di Sergio | 0.6 | 0.7 |
|---|---|---|
| Traduttore veloce (Mozilla) | 69,1 · 11 ms | **70,3** · 20 ms (modello 2.1 di Firefox) |
| Veloce + correttore IA tutto sulla scheda | 70,7 (Qwen3-8B) | **73,0** (EuroLLM-9B) · 0,37 s · 5,2 GB |
| IA divisa scheda/processore (200 frasi) | — | 76,2 contro 74,2 di Mozilla · 1,2 s · 3,6 GB |

- Modello inglese→italiano 2.1 di Mozilla (quello di Firefox, MPL-2.0) e italiano→inglese 2.0 per la chat.
- Correttore IA: **EuroLLM-9B-Instruct** (Apache-2.0, progetto europeo UTTER, fatto per tradurre)
  con istruzioni da "localizzatore professionista". Scelto in modo onesto: provato sulla metà delle
  1000 frasi (id dispari), misurato una volta sola sull'altra metà mai usata (id pari): Mozilla 70,18
  → EuroLLM 72,98, Qwen3-8B 71,64. EuroLLM peggiora molte meno frasi (52 contro 92 su 500).
- Regole della memoria video invariate (tutto sulla scheda da 6,5 GB liberi, diviso da 4,9 GB e 8 core).
- Provati e scartati: prompt diversi per Qwen3 (nessun guadagno), battute precedenti nel prompt
  (peggio), "traduci tu con la bozza come aiuto" (72,80, poco sotto).

## Versione 0.6 (22/11/2026, sera): sotto mezzo secondo

Misure dal vivo sul PC di Sergio (exe di collaudo, stesso codice dell'exe 0.6):

| Cosa | Prima | Adesso |
|---|---|---|
| Voce → sottotitolo italiano (dalla fine della voce) | 0,8 s | **0,3-0,4 s**, anche con musica sotto (65-66 parole su 67) |
| Cambio schermata → traduzione disegnata | oltre 1 s | **0,43-0,68 s** (40 scritte) |
| Lettura del testo (OCR) | 130 ms | 60-110 ms |
| Traduzione di 40 scritte | circa 300 ms | 1-140 ms (insieme, più traduttori Mozilla) |
| 1000 frasi | 69,1 / con IA 70,7 | uguale (nessun peggioramento) |

Come: la voce si comincia a capire alla prima pausa di 0,15 s (prima si aspettavano 0,45 s) e i
pezzi della stessa frase si riattaccano; whisper con contesto di 7,7 s su tutti i core a priorità
bassa; il rumore di fondo si impara in 3 s (con la musica continua prima la musica sembrava voce);
le scritte di una schermata si traducono tutte insieme con il traduttore "a gruppi" di Bergamot;
l'immagine va all'OCR senza compressione; lo schermo si controlla ogni 60 ms.

Onestamente: nel cambio schermata quasi metà del tempo è la "foto" dello schermo (circa 250 ms con
BitBlt). Durante le prove i monitor erano probabilmente in risparmio energetico (Windows aggiorna lo
schermo pochi volte al secondo e la duplicazione del desktop non mandava fotogrammi). È stata
aggiunta la cattura con la duplicazione del desktop (DXGI, come OBS), che si attiva da sola solo se
funziona davvero: va verificata con il monitor acceso e un gioco vero.

## Versione 0.5 (22/11/2026, pomeriggio)

- Voci più veloci: whisper con contesto audio di 10 s invece di 30 (`-ac 512`): 1,1 s → 0,45 s a
  battuta sul Ryzen 7 3800XT, stessa precisione. Sottotitolo finale **0,8-0,9 s dopo la fine della
  voce** (prima 1,5 s); sottotitolo **provvisorio** mentre il personaggio parla ancora (circa 2 s
  dopo l'inizio della frase, con "…"). Collaudo: 66 parole su 67 con musica sotto a -10 dB.
- Trovati con le 10 foto del programma vero e corretti: parte del gioco non tradotta se la finestra
  di Tasso stava dietro al gioco; percentuali lette male dall'OCR ("6.796" → "6.7%", "IS%" →
  "15%"); G letta come C ("Crowth" → "Growth"); tasti che andavano a capo nella finestra; scheda
  del correttore IA che restava su "Scarica" anche a IA installata.
- 1000 frasi senza peggioramenti: Mozilla 69,1 (11 ms), con IA 70,7.
- Foto: `TassoTraduce-collaudo.exe --foto <cartella> --scene <immagini di gioco> --voci-prova <audio>`.

## Stato onesto della versione 0.4 (22/11/2026)

Controllata da 5 revisori indipendenti (correttezza, sicurezza e regole, robustezza, interfaccia,
scettico): circa 30 errori corretti. Nessuna violazione delle regole: non tocca file né memoria dei
giochi, non manda dati, scarica solo file con impronta dopo il sì. In più: chiave segreta per il
motore IA, file dei motori controllati prima dell'avvio, modalità di prova solo nell'exe di collaudo
(`go build -tags collaudo`).

Numeri dal vivo sul PC di Sergio (exe 0.4, misura corretta):

| Prova | Risultato |
|---|---|
| 1000 frasi, Mozilla | 69,1 (prima delle correzioni 68,1) · 11 ms |
| 1000 frasi, Mozilla + IA tutta sulla scheda | 70,7 · 0,35 s · 4,6 GB |
| 200 frasi, IA divisa 24/36 | 74,3 contro 74,2 tutta sulla scheda · 1,25 s · 3,2 GB |
| Voci, voce sintetica pulita | 67 parole su 67 · sottotitolo 1,0-1,5 s dopo la fine della voce |
| Voci, con musica sotto a -10 dB | 65 parole su 67 · 1,5 s dopo la fine della voce |
| Chat italiano→inglese | 11-24 ms a frase, appunti ok |

Avvertenze (dallo scettico):
- Circa metà del +1,0 di Mozilla viene da regole scritte guardando le stesse 1000 frasi: su testi
  nuovi conterà meno. Gli accenti ("E'"→"È") invece valgono ovunque. 9 frasi del test sono anche
  nel catalogo dei menu.
- L'IA migliora 10 generi su 14; peggiora un po' horror, noir e umorismo; delle frasi che cambia,
  circa 4 su 10 vengono peggio.
- Niente è stato provato con un gioco acceso: tempi, memoria e "0 pause" valgono a PC fermo.
- Voci provate con voce sintetica e musica finta, non con un gioco vero. Il sottotitolo non è
  stato visto sullo schermo (le finestre di Tasso sono escluse dalle catture).
- Grassetto provato solo su scritte disegnate da Windows.

## Novità 0.3 (22/11/2026)

### Voci dei personaggi in italiano (sottotitoli dal vivo)

Interruttore "Voci dei personaggi in italiano" nella finestra, oppure **Ctrl+Alt+V** nel gioco.
Tasso ascolta l'audio del PC (WASAPI "loopback", come la registrazione dello schermo: non tocca
il gioco), taglia le battute nelle pause, le fa capire a **whisper.cpp** sul PC (MIT, modello
OpenAI Whisper base inglese, MIT, con rilevatore di voce Silero, MIT), le traduce con Mozilla (e
il correttore IA, se c'è) e mostra il sottotitolo italiano in basso sulla finestra in primo piano.
Funziona anche con filmati e video. Si scarica una volta sola, dopo il sì (circa 150 MB).

Collaudo dal vivo sul PC di Sergio: 5 frasi fatte uscire dalle casse, **67 parole su 67 capite**,
sottotitolo italiano circa **1 s** dopo la chiusura della battuta (più 0,45 s di pausa che serve per
capire che la frase è finita). Provato con una voce sintetica di Windows, non ancora con un gioco vero
con musica ed effetti sopra.

### Chat in inglese

Scrivi in italiano nella chat del gioco, **Ctrl+A**, **Ctrl+X**, poi **Ctrl+Alt+T**: l'inglese è
negli appunti, **Ctrl+V** e invii. Oppure scrivi nella casella "Chat del gioco in inglese". Motore
Mozilla italiano→inglese (MPL-2.0, 22 MB, impronte dal registro ufficiale Bergamot). Collaudo: 8
frasi da chat tradotte in 11-24 ms, appunti con accenti giusti, tasto provato.

## Collaudo dal vivo del 22/11/2026 (PC di Sergio: Ryzen 7 3800XT, 48 GB, RX 6700 XT 12 GB)

Fatto con `TassoTraduce.exe --collaudo "C:\cartella" [--anche-meta 200]`: il programma vero, Mozilla
dentro WebView2, IA con llama.cpp Vulkan, senza toccare memoria e librerie dell'utente.

| Prova | Risultato |
|---|---|
| 1000 frasi, Mozilla (prima delle correzioni) | chrF++ 68,1 · 11 ms a frase |
| 1000 frasi, Mozilla con le correzioni del 22/11 | chrF++ 69,1 · 11 ms a frase |
| 1000 frasi, Mozilla + IA Qwen3-8B tutta sulla scheda | chrF++ 70,1 · 0,37 s a correzione · 4,6 GB di scheda · 0 pause |
| 200 frasi, IA divisa (24 strati su 36) | stessa qualità della scheda intera (71,9 contro 71,8) · 1,4 s · 3,2 GB |
| Regola automatica | 9,8 GB liberi, 48 GB, 8 core veri → "IA tutta sulla scheda" (giusto) |
| Grassetto con l'OCR vero di Windows | 8 scritte su 8 riconosciute giuste |
| Termini noti (Elmo di ferro, Salvataggio automatico, Maiusc…) | 14 su 16 (restano "caschi"/"casco" senza materiale: in fantascienza è giusto) |

L'IA migliora 10 generi su 14; peggiora un po' horror (-1,3), menu (-1,5), noir (-0,8), umorismo (-0,3).

Corretto il 22/11: core veri invece dei thread; RAM installata vera (i PC da 16 GB non restano
esclusi); il modo diviso controlla anche la RAM libera; il lavoro dell'IA non conta come "processore
pieno"; correzione con temperatura 0; la correzione arriva subito anche sulla scritta già sullo
schermo; accenti di Mozilla ("E'", "pero'", "servira'"); termini da gioco nelle frasi
(`termini_frasi.go`, solo errori visti nel collaudo); scritte di menu ("Cooldown: 12 secondi") non
più scambiate per nomi; soglia di somiglianza per gli errori dell'OCR (`quasi_uguale.go`); risposte
"incantate" dell'IA scartate; avviso per i giochi a schermo intero esclusivo (immagine nera);
Steamworks non più scambiato per un gioco.

## Ancora da fare (onestamente)

- **Non ancora provato su un gioco vero** con questa versione.
- Le misure di velocità dell'IA sono a PC fermo: con un gioco pesante aperto vanno rifatte.
- Icona e manifest dentro l'exe; firma digitale prima di metterlo sul sito.
- Idee dai concorrenti ancora da fare: correggere a mano dalla cronologia, tasti del controller,
  tasto "tieni premuto per vedere l'inglese", più zone.
- Voci: modello base inglese su processore (whisper.cpp per Windows non ha la versione Vulkan);
  con giochi molto rumorosi la qualità va misurata.

## Licenze

Codice: GPL-3.0, Sergio Tassoni. `third_party/go-webview2`: MIT (copia locale di Tasso).
`ui/bergamot`: Bergamot translator, MPL-2.0, sorgente su https://github.com/browsermt/bergamot-translator.
Modelli inglese→italiano: Mozilla, MPL-2.0.
