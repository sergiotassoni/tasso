"""Genera il manuale d'uso di Tasso in PDF (A4)."""
import sys
from reportlab.lib.pagesizes import A4
from reportlab.lib.units import mm
from reportlab.lib import colors
from reportlab.lib.styles import ParagraphStyle
from reportlab.lib.enums import TA_CENTER
from reportlab.platypus import (BaseDocTemplate, PageTemplate, Frame, Paragraph, Spacer, Table, TableStyle,
                                Image, PageBreak, KeepTogether, NextPageTemplate, CondPageBreak)
from reportlab.pdfbase import pdfmetrics
from reportlab.pdfbase.ttfonts import TTFont

OUT, LOGO = sys.argv[1], sys.argv[2]
import os
GF = os.environ.get('TASSO_FONTS', '/usr/share/fonts/truetype/google-fonts/').rstrip('/') + '/'
pdfmetrics.registerFont(TTFont('Pop', GF + 'Poppins-Regular.ttf'))
pdfmetrics.registerFont(TTFont('PopB', GF + 'Poppins-Bold.ttf'))
pdfmetrics.registerFont(TTFont('PopM', GF + 'Poppins-Medium.ttf'))
pdfmetrics.registerFont(TTFont('PopL', GF + 'Poppins-Light.ttf'))
pdfmetrics.registerFont(TTFont('PopI', GF + 'Poppins-Italic.ttf'))
from reportlab.pdfbase.pdfmetrics import registerFontFamily
registerFontFamily('Pop', normal='Pop', bold='PopB', italic='PopI', boldItalic='PopB')

INK = colors.HexColor('#121522')
MUTED = colors.HexColor('#5A6075')
GREEN = colors.HexColor('#0FA46E')
GREEN_BG = colors.HexColor('#E7F8F1')
VIOLET = colors.HexColor('#6E48D8')
VIOLET_BG = colors.HexColor('#F0EBFF')
AMBER = colors.HexColor('#B86E00')
AMBER_BG = colors.HexColor('#FFF4E0')
RED = colors.HexColor('#C9372C')
RED_BG = colors.HexColor('#FDECEA')
BLUE = colors.HexColor('#2563C9')
LINE = colors.HexColor('#E3E6EF')
DARK = colors.HexColor('#07080D')

W, H = A4
M = 18 * mm

st = {
    'h1': ParagraphStyle('h1', fontName='PopB', fontSize=22, leading=27, textColor=INK, spaceBefore=4, spaceAfter=8),
    'h2': ParagraphStyle('h2', fontName='PopB', fontSize=13.5, leading=18, textColor=INK, spaceBefore=12, spaceAfter=4),
    'p': ParagraphStyle('p', fontName='Pop', fontSize=10, leading=15, textColor=INK, spaceAfter=6),
    'small': ParagraphStyle('small', fontName='Pop', fontSize=8.8, leading=12.5, textColor=MUTED),
    'li': ParagraphStyle('li', fontName='Pop', fontSize=10, leading=14.5, textColor=INK, leftIndent=14, bulletIndent=2, spaceAfter=3),
    'num': ParagraphStyle('num', fontName='Pop', fontSize=10, leading=14.5, textColor=INK, leftIndent=18, bulletIndent=0, spaceAfter=4),
    'cell': ParagraphStyle('cell', fontName='Pop', fontSize=9, leading=12.5, textColor=INK),
    'cellb': ParagraphStyle('cellb', fontName='PopB', fontSize=9, leading=12.5, textColor=INK),
    'kicker': ParagraphStyle('kicker', fontName='PopB', fontSize=9, leading=12, textColor=GREEN, spaceAfter=2),
    'toc': ParagraphStyle('toc', fontName='Pop', fontSize=11, leading=20, textColor=INK),
}


def P(t, s='p'):
    return Paragraph(t, st[s])


def bullets(items, s='li'):
    return [Paragraph(x, st[s], bulletText='•') for x in items]


def steps(items):
    return [Paragraph(x, st['num'], bulletText='%d.' % (i + 1)) for i, x in enumerate(items)]


def box(title, body, fg, bg):
    inner = [Paragraph('<font name="PopB" color="%s">%s</font>' % (fg.hexval().replace('0x', '#'), title), st['p'])]
    for b in body if isinstance(body, list) else [body]:
        inner.append(b if not isinstance(b, str) else Paragraph(b, st['p']))
    t = Table([[inner]], colWidths=[W - 2 * M])
    t.setStyle(TableStyle([
        ('BACKGROUND', (0, 0), (-1, -1), bg),
        ('LINEBEFORE', (0, 0), (0, -1), 3, fg),
        ('LEFTPADDING', (0, 0), (-1, -1), 12), ('RIGHTPADDING', (0, 0), (-1, -1), 12),
        ('TOPPADDING', (0, 0), (-1, -1), 9), ('BOTTOMPADDING', (0, 0), (-1, -1), 5),
    ]))
    return KeepTogether([Spacer(1, 4), t, Spacer(1, 8)])


def table(rows, widths, head_bg=INK):
    th = ParagraphStyle('th', parent=st['cellb'], textColor=colors.white)
    data = [[Paragraph(c, th if i == 0 else st['cell']) for c in r] for i, r in enumerate(rows)]
    t = Table(data, colWidths=widths, repeatRows=1)
    t.setStyle(TableStyle([
        ('BACKGROUND', (0, 0), (-1, 0), head_bg),
        ('ROWBACKGROUNDS', (0, 1), (-1, -1), [colors.white, colors.HexColor('#F6F7FB')]),
        ('LINEBELOW', (0, 0), (-1, -1), 0.5, LINE),
        ('VALIGN', (0, 0), (-1, -1), 'TOP'),
        ('LEFTPADDING', (0, 0), (-1, -1), 7), ('RIGHTPADDING', (0, 0), (-1, -1), 7),
        ('TOPPADDING', (0, 0), (-1, -1), 6), ('BOTTOMPADDING', (0, 0), (-1, -1), 6),
    ]))
    return KeepTogether([Spacer(1, 4), t, Spacer(1, 10)])


def chapter(n, title, kicker):
    return [CondPageBreak(90 * mm), Spacer(1, 6), P(('CAPITOLO %d' % n) + ('  ·  ' + kicker if kicker else ''), 'kicker'), P(title, 'h1')]


# ---------------- pagine ----------------

def cover(c, doc):
    c.saveState()
    c.setFillColor(DARK)
    c.rect(0, 0, W, H, fill=1, stroke=0)
    c.setFillColor(colors.HexColor('#1B1240'))
    c.circle(W - 40 * mm, H - 30 * mm, 95 * mm, fill=1, stroke=0)
    c.setFillColor(colors.HexColor('#0C2A20'))
    c.circle(30 * mm, 40 * mm, 80 * mm, fill=1, stroke=0)
    c.drawImage(LOGO, W / 2 - 32 * mm, H / 2 + 8 * mm, 64 * mm, 64 * mm, mask='auto')
    c.setFillColor(colors.white)
    c.setFont('PopB', 54)
    c.drawCentredString(W / 2, H / 2 - 18 * mm, 'TASSO')
    c.setFont('PopM', 13)
    c.setFillColor(colors.HexColor('#2FF3A6'))
    c.setFillColor(colors.HexColor('#C9CDE0'))
    c.setFont('Pop', 15)
    c.drawCentredString(W / 2, H / 2 - 48 * mm, "Manuale d'uso")
    c.setFont('PopL', 11)
    c.drawCentredString(W / 2, H / 2 - 57 * mm, 'Pulisce, ripara e spinge il tuo PC. In un clic.')
    c.setFont('Pop', 9)
    c.setFillColor(colors.HexColor('#8B91A7'))
    c.drawCentredString(W / 2, 18 * mm, 'Versione 3.0  ·  Windows 10 e 11  ·  2026')
    c.restoreState()


def page(c, doc):
    c.saveState()
    c.setStrokeColor(LINE)
    c.setLineWidth(0.6)
    c.line(M, H - 12 * mm, W - M, H - 12 * mm)
    c.setFont('PopB', 8.5)
    c.setFillColor(INK)
    c.drawString(M, H - 10 * mm, 'TASSO')
    c.setFont('Pop', 8.5)
    c.setFillColor(MUTED)
    c.drawString(M + 14 * mm, H - 10 * mm, "Manuale d'uso")
    c.drawRightString(W - M, 10 * mm, str(doc.page))
    c.restoreState()


doc = BaseDocTemplate(OUT, pagesize=A4, leftMargin=M, rightMargin=M, topMargin=18 * mm, bottomMargin=17 * mm,
                      title="Tasso - Manuale d'uso", author='Tasso', subject='Manuale di Tasso 2.6')
frame = Frame(M, 17 * mm, W - 2 * M, H - 35 * mm, id='f', leftPadding=0, rightPadding=0, topPadding=0, bottomPadding=0)
doc.addPageTemplates([PageTemplate(id='cover', frames=[frame], onPage=cover), PageTemplate(id='body', frames=[frame], onPage=page)])

S = []
S += [NextPageTemplate('body'), PageBreak()]

# Indice
S += [P('Indice', 'h1'), Spacer(1, 4)]
toc = [
    ('1', 'Primi passi', 'aprire Tasso, Gaming o Ufficio, l\'avviso di Windows'),
    ('2', 'La Home', 'il voto del PC, il pulsante PULISCI, i sensori'),
    ('3', 'Le 5 pulizie', 'Classica, Normale, Media, Profonda, Eccezionale'),
    ('4', 'Velocità', 'modalità, Turbo, programmi all\'avvio'),
    ('5', 'Programmi', 'disinstallare per bene, app preinstallate'),
    ('6', 'File', 'doppi, grandi, cartelle vuote, distruggi e recupera'),
    ('7', 'Risolvi problemi', 'i 100 problemi dell\'ufficio, i giochi e Windows'),
    ('8', 'Internet, disco e giochi', 'ping, velocità di Internet, disco, giochi'),
    ('9', 'Hardware', 'componenti, sensori, BIOS, scheda video'),
    ('10', 'Impostazioni', 'temi, avvio con Windows, avvisi'),
    ('11', 'Spegnere Tasso e rimettere com\'era', 'l\'interruttore e il ripristino'),
    ('12', 'Sicurezza e backup', 'come tornare indietro se serve'),
    ('13', 'Domande frequenti', 'antivirus, chiavetta, mandarlo a un amico'),
]
rows = [[Paragraph('<font name="PopB" color="#0FA46E">%s</font>' % n, st['toc']), Paragraph('<b>%s</b>' % t, st['toc']), Paragraph(d, st['small'])] for n, t, d in toc]
tt = Table(rows, colWidths=[12 * mm, 62 * mm, W - 2 * M - 74 * mm])
tt.setStyle(TableStyle([('LINEBELOW', (0, 0), (-1, -1), 0.5, LINE), ('VALIGN', (0, 0), (-1, -1), 'MIDDLE'), ('TOPPADDING', (0, 0), (-1, -1), 4), ('BOTTOMPADDING', (0, 0), (-1, -1), 4)]))
S += [tt, Spacer(1, 10)]
S += [box('In breve', ['Tasso è un programma gratuito che tiene il PC pulito, veloce e in ordine. Usa gli strumenti ufficiali di Microsoft, prima delle pulizie importanti fa un <b>backup automatico</b>, e non cancella mai i tuoi file personali senza chiedertelo: quello che scegli di eliminare va nel <b>Cestino</b>.'], GREEN, GREEN_BG)]

# 1 Primi passi
S += chapter(1, 'Primi passi', 'per iniziare')
S += [P('<b>Cosa serve</b>', 'h2')]
S += bullets(['Un PC con <b>Windows 10 o Windows 11</b> a 64 bit.',
              '<b>Microsoft Edge WebView2</b>: è già dentro Windows 11 e quasi tutti i Windows 10. Se manca, Tasso apre da solo la pagina ufficiale Microsoft per installarlo gratis.',
              'I permessi di <b>amministratore</b> del PC (quelli che hai di solito sul tuo computer di casa).'])
S += [P('<b>Come si apre</b>', 'h2')]
S += steps(['Scarica <b>Tasso.zip</b> da github.com/sergiotassoni/tasso, fai tasto destro sul file e scegli <b>Estrai tutto</b>.',
            'Apri la cartella <b>Tasso</b> e fai doppio clic su <b>Tasso.exe</b>.',
            'Windows chiede "Vuoi consentire a questa app di apportare modifiche?": rispondi <b>Sì</b>. Serve per pulire e riparare il sistema.',
            'Se compare la schermata blu <b>"PC protetto da Windows"</b>, clicca <b>Ulteriori informazioni</b> e poi <b>Esegui comunque</b> (vedi il riquadro qui sotto).'])
S += [box('Perché Windows dice "PC protetto"?', ['Tasso ha la <b>firma digitale</b> (la vedi in Proprietà &gt; Firme digitali), ma Windows mostra questo avviso per i programmi che non sono ancora stati scaricati da abbastanza persone. Non significa che Tasso sia un virus: Windows semplicemente non lo conosce ancora. Più persone lo usano, meno spesso compare.'], AMBER, AMBER_BG)]
S += [P('<b>La prima configurazione</b>', 'h2'),
      P('La prima volta Tasso ti chiede <b>come userai il PC</b>. Tocca la scheda giusta (Tasso ti fa vedere subito come sarà) e poi <b>Continua</b>:')]
S += [table([['Modalità', 'Com\'è Tasso e cosa imposta'],
             ['<b>Gaming</b>', 'Tutto Tasso: giochi, FPS, ping ai server, Turbo, Prestazioni+, temi scuri e colorati. Mette la modalità Gaming del PC.'],
             ['<b>Ufficio</b>', 'Più semplice e tranquillo: colori chiari e morbidi, un tasso più carino, niente giochi, FPS e ping. In Risolvi problemi trovi subito i <b>100 problemi più comuni dell\'ufficio</b>. Mette la modalità Normale e la pulizia programmata ogni settimana.']],
            [32 * mm, W - 2 * M - 32 * mm])]
S += [P('Puoi cambiare modalità quando vuoi con il pulsante <b>Gaming / Ufficio</b> in alto a sinistra, sotto il logo (o da <b>Impostazioni</b>). Il pulsante cambia solo come appare Tasso, non tocca il PC. Ogni modalità ricorda il suo tema.', 'small')]

# 2 Home
S += chapter(2, 'La Home', 'colpo d\'occhio')
S += bullets(['<b>Il voto del PC</b> (da 0 a 100%): tiene conto dei file inutili, dello spazio libero, dei programmi all\'avvio, dello stato dei dischi e di quanto tempo è passato dall\'ultima pulizia.',
              '<b>Salute del PC</b>: il voto da 0 a 100 con i consigli per migliorarlo (vedi il capitolo 8).',
              '<b>Il pulsante rotondo PULISCI</b>: avvia la pulizia consigliata (la Media). Prima ti mostra cosa farà e ti chiede conferma.',
              '<b>Scegli la potenza</b>: le cinque schede per scegliere la pulizia che vuoi (vedi il capitolo 3).',
              '<b>Pulizia programmata</b>: sotto le pulizie. Scegli il giorno, l\'ora, il livello (Classica, Normale o Media; se vuoi anche Profonda o Eccezionale, che durano di più) e se ogni settimana o ogni 2 settimane. Se a quell\'ora il PC è spento, Tasso pulisce appena lo riaccendi. Nella pulizia programmata Tasso è più prudente: dal Cestino toglie solo quello buttato da più di 7 giorni, e i lavori lunghi li salta se il portatile va a batteria.',
              '<b>I sensori</b> in basso: quanto lavora il processore, la RAM usata, quanto è pieno il disco e, con le schede NVIDIA, la temperatura della scheda video.',
              '<b>La modalità attiva</b> è scritta in alto a destra (Normale, Gaming, FPS, Film, Silenzioso).'])

# 3 Pulizie
S += chapter(3, 'Le 5 pulizie', 'il cuore di Tasso')
S += [table([['Pulizia', 'Durata', 'Cosa fa', 'Quando'],
             ['<b>1 Classica</b>', '1 min', 'I tuoi file temporanei, il Cestino e le miniature delle foto.', 'Quando vuoi, anche ogni giorno.'],
             ['<b>2 Normale</b>', '3 min', 'Tutto la Classica + file temporanei di Windows, cache dei browser, cache DNS.', 'Ogni settimana.'],
             ['<b>3 Media</b>', '16 min', 'Tutto la Normale + <b>backup</b>, cache delle app (Discord, Steam, Teams, Epic) e della scheda video, cache di Windows Update, file di sistema inutili, resti dei driver, cache dei launcher. È la <b>consigliata</b>.', 'Ogni 2 settimane.'],
             ['<b>4 Profonda</b>', '55 min', 'Tutto la Media + driver e aggiornamenti di Windows, driver video vecchi, programmi all\'avvio, estensioni dei browser, salute dei dischi, sicurezza, deframmentazione o TRIM, antivirus.', 'Una volta al mese.'],
             ['<b>5 Eccezionale</b>', '90 min', 'Tutto la Profonda + riparazione di Windows (DISM e SFC), stabilità (crash ed errori hardware), controllo di ogni programma, errori del disco, periferiche.', 'Quando il PC dà problemi o ogni 2-3 mesi.']],
            [28 * mm, 17 * mm, 90 * mm, W - 2 * M - 135 * mm])]
S += [box('Il voto al PC, prima e dopo', ['In <b>Pulizia</b> trovi il <b>Voto al PC</b>: misura in pochi secondi la velocità di processore, memoria e disco e quanto è "leggero" Windows (memoria occupata, programmi in background e all\'avvio, spazio libero, file inutili). Tutti i voti vanno <b>da 1 a 100</b> (70 = un PC da gioco medio di oggi) e c\'è un <b>voto totale</b> con la <b>classifica</b>: PC lento (sotto 40), PC da ufficio (40-59), buon PC (60-79), PC veloce (80-94), top (95 e più). Se accendi "Misura prima e dopo ogni pulizia" vedi quanto è servita. Onestamente: la pulizia migliora soprattutto la leggerezza; la velocità dei pezzi dipende dall\'hardware.'], GREEN, GREEN_BG)]
S += [P('<b>Durante la pulizia</b>', 'h2')]
S += bullets(['La barra si muove e le frasi cambiano: il PC sta lavorando, puoi anche andare a fare altro.',
              'A destra vedi lo <b>spazio liberato</b>, il <b>tempo che manca</b> e i <b>dettagli</b> di ogni passaggio.',
              '<b>Interrompi</b>: Tasso si ferma appena finisce il passaggio in corso, così non lascia niente a metà.',
              '<b>Spegni il PC quando ha finito</b>: spunta la casella prima di avviare. Alla fine il PC si spegne dopo 2 minuti, e puoi annullare con il pulsante <b>Annulla lo spegnimento</b>.',
              'Chiudi i browser prima di iniziare: quelli aperti vengono saltati.'])
S += [box('Al termine', ['Vedi lo spazio liberato, la durata e il registro completo. Se sono stati installati aggiornamenti o fatte riparazioni, Tasso ti propone di <b>riavviare</b>. Ogni pulizia viene salvata in un registro (Impostazioni, <b>Apri la cartella dei registri</b>).'], GREEN, GREEN_BG)]

# 4 Velocità
S += chapter(4, 'Velocità', 'più scattante')
S += [P('<b>Le modalità</b>', 'h2')]
S += [table([['Modalità', 'Cosa fa'],
             ['<b>Normale</b>', 'Equilibrio tra velocità e consumi, notifiche attive. Va bene tutti i giorni.'],
             ['<b>Gaming</b>', 'Piano energetico al massimo, Modalità Gioco di Windows attiva, notifiche in pausa.'],
             ['<b>FPS competitivo</b>', 'Come Gaming, più il mouse senza accelerazione per una mira più precisa.'],
             ['<b>Film</b>', 'Niente notifiche e lo schermo non si spegne durante il film.'],
             ['<b>Silenzioso</b>', 'Processore limitato al 70%: meno calore e ventole più tranquille.']],
            [36 * mm, W - 2 * M - 36 * mm])]
S += [P('Prima di attivare una modalità, Tasso controlla il PC e ti avvisa se serve: per esempio se sei su un portatile (meglio tenere il caricatore attaccato) o se hai poca RAM.', 'p')]
S += [P('<b>Turbo</b>', 'h2'),
      P('Chiude i programmi in background che non servono mentre giochi (OneDrive, Teams, Skype, Dropbox e simili) e libera memoria. Non tocca i tuoi giochi, Discord o il browser. I programmi chiusi si riaprono da soli al prossimo avvio del PC.')]
S += [P('<b>Programmi all\'avvio</b>', 'h2'),
      P('Ogni programma che parte quando accendi il PC ha il suo <b>interruttore</b>. Spegni quelli che non ti servono subito: Windows si accende più in fretta. Il programma resta installato, semplicemente non parte da solo.'),
      P('<b>Guardiano dell\'avvio</b>: se un programma nuovo si aggiunge all\'avvio di nascosto, Tasso te lo segnala con un piccolo avviso.', 'p')]

# 5 Programmi
S += chapter(5, 'Programmi', 'installa e togli')
S += [P('<b>Disinstallare per bene</b>', 'h2')]
S += steps(['Vai su <b>Programmi</b> e cerca il programma (puoi scriverne il nome nella barra di ricerca).',
            'Clicca <b>Disinstalla</b> e segui le finestre del programma di disinstallazione.',
            'Alla fine Tasso cerca le <b>cartelle rimaste</b> e ti chiede se spostarle nel Cestino.'])
S += [P('<b>App preinstallate</b>', 'h2'),
      P('Sono i giochini e le app pubblicitarie che Windows installa da solo (Candy Crush, TikTok, Solitaire, Notizie e simili). Quelle già spuntate sono inutili per quasi tutti; le altre toglile solo se non le usi. Se te ne serve di nuovo una, la reinstalli gratis dal Microsoft Store.')]

# 6 File
S += chapter(6, 'File', 'ordine e spazio')
S += steps(['Vai su <b>File</b> e clicca <b>Analizza i miei file</b>. Tasso guarda Desktop, Documenti, Immagini, Video, Musica e Download.',
            'Ti mostra <b>file doppi</b> (il primo di ogni gruppo resta, le copie sono spuntate), <b>file più grandi</b> (scegli tu: sopra 100 MB, 300 MB o 1 GB; quelli non toccati da più di un anno sono segnati), <b>cartelle vuote</b> e <b>collegamenti rotti</b>. Con <b>Aggiungi una cartella</b> controlli anche un\'altra cartella o un disco esterno.',
            'Controlla le spunte e clicca <b>Sposta nel Cestino i selezionati</b>. Fino a quando non svuoti il Cestino puoi recuperare tutto.'])
S += [table([['Strumento', 'A cosa serve'],
             ['<b>Distruggi file</b>', 'Per documenti riservati, anche più file insieme: vengono sovrascritti e cancellati, e <b>non si possono più recuperare</b>. Onestamente: sugli SSD Windows sposta i dati da solo, quindi la garanzia totale non esiste; Tasso chiede anche al disco di cancellare subito gli spazi liberi (TRIM). Per un PC da vendere usa anche Cancella lo spazio libero.'],
             ['<b>Recupera file cancellati</b>', 'Apre il Cestino e lo strumento gratuito di Microsoft con la sua guida. Consiglio: non salvare nuovi file finché non hai recuperato quelli che ti servono.'],
             ['<b>Cancella lo spazio libero</b>', 'Rende impossibile recuperare i file già cancellati in passato. Utile prima di vendere o regalare il PC. Può durare ore.']],
            [44 * mm, W - 2 * M - 44 * mm])]

# 7 Risolvi
S += chapter(7, 'Risolvi problemi', 'un clic sul problema')
S += [table([['Problema', 'Cosa fa Tasso'],
             ['<b>Non sento l\'audio</b>', 'Riavvia i servizi audio di Windows e controlla le schede audio.'],
             ['<b>Internet non va</b>', 'Svuota la cache, rinnova la connessione e ripristina le impostazioni di rete. Internet si stacca per qualche secondo; alla fine serve un riavvio.'],
             ['<b>Windows Update bloccato</b>', 'Ripristina i componenti di Windows Update quando gli aggiornamenti non si installano.'],
             ['<b>Windows dà errori</b>', 'Controlla e ripara i file di sistema (DISM e SFC). 20-40 minuti.'],
             ['<b>Una periferica non va</b>', 'Controlla mouse, tastiera, rete, video e USB e ti dice quale ha problemi.'],
             ['<b>Il disco fa i capricci</b>', 'Controlla errori, salute, usura e temperatura dei dischi.'],
             ['<b>Stampante, Store, menu Start</b>', 'Svuota la coda di stampa bloccata, ripara lo Store e le app, riavvia ricerca e menu Start.']],
            [46 * mm, W - 2 * M - 46 * mm])]
S += [P('<b>Problemi nei giochi</b>', 'h2'),
      P('I 13 problemi più comuni di chi gioca, ognuno con il suo pulsante. Tasso spiega sempre cosa fa e non tocca mai giochi, salvataggi o file dell\'anti-cheat.')]
S += [table([['Problema', 'Cosa fa Tasso'],
             ['<b>Il gioco va a scatti</b>', 'Svuota le cache della grafica (gli "shader" di DirectX, AMD, NVIDIA, Intel), accende la Modalità Gioco e ti dice se il PC sta risparmiando energia.'],
             ['<b>Pochi FPS</b>', 'Gli FPS sono le immagini al secondo. Mette la potenza al massimo, spegne la registrazione nascosta e ti dice cosa ruba potenza (batteria, memoria piena, programmi pesanti).'],
             ['<b>Ping alto o lag online</b>', 'Svuota la memoria degli indirizzi internet, rimette la rete come la vuole Windows, ferma la condivisione nascosta degli aggiornamenti e ti avvisa se il Wi-Fi ti rallenta.'],
             ['<b>Il gioco non parte</b>', 'Installa o ripara Visual C++, DirectX e .NET dal sito ufficiale Microsoft: sono i "pezzi" di Windows che servono ai giochi.'],
             ['<b>Steam, Epic e launcher</b>', 'Chiude i launcher bloccati e svuota le loro pagine salvate (schermate bianche o nere). Giochi e account restano.'],
             ['<b>Schermo nero o risoluzione</b>', 'Fa ripartire la scheda video in 2 secondi e ti spiega i tasti utili (Alt+Invio, Win+P).'],
             ['<b>Controller, cuffie, microfono</b>', 'Riavvia le periferiche bloccate e l\'audio di Windows, e controlla se il microfono è bloccato dalla privacy.'],
             ['<b>Xbox e Game Pass</b>', 'Riavvia i servizi Xbox e ripara l\'app Xbox e lo Store.'],
             ['<b>Errore dell\'anti-cheat</b>', 'Controlla Vanguard, Easy Anti-Cheat, BattlEye e FACEIT e li riaccende se un altro programma li ha spenti.'],
             ['<b>Discord</b>', 'Chiude Discord bloccato, svuota la sua cache (messaggi e accesso restano) e controlla il microfono.'],
             ['<b>La mira con il mouse non è precisa</b>', 'Spegne l\'"accelerazione" del puntatore di Windows: lo stesso movimento sposta la mira sempre della stessa quantità. Si torna indietro con un doppio clic.'],
             ['<b>Il gioco si riduce a icona da solo</b>', 'Rimette la protezione di Windows che impedisce agli altri programmi di "rubare" la finestra e ti dice quali programmi aperti lo fanno di solito.'],
             ['<b>I download dei giochi sono lenti</b>', 'Controlla rete "a consumo", limiti di velocità, aggiornamenti e disco pieno, e svuota la cache dei download di Steam. Giochi e salvataggi restano.']],
            [46 * mm, W - 2 * M - 46 * mm])]
S += [P('<b>I 100 problemi dell\'ufficio</b> (novità della 2.6)', 'h2'),
      P('Risolvi problemi è diviso in tre sezioni: <b>Ufficio</b>, <b>Gaming</b> (solo nella modalità Gaming) e <b>Windows</b>, con <b>una ricerca sola</b> in cima: scrivi il problema (per esempio "stampante", "zip" o "password") e Tasso ti mostra solo quelli giusti, in tutte le sezioni. I 100 problemi dell\'ufficio sono divisi in 10 gruppi che si aprono con un tocco (stampa, Word/Excel/Adobe, Outlook, Teams, file e zip, rete, schermi, mouse e tastiera, Windows, account e sicurezza). In Ufficio i gruppi sono aperti e la sezione è la prima; in Gaming i gruppi sono chiusi e la sezione è dopo quelle dei giochi e di Windows. Tasso non cancella mai documenti (al massimo li sposta in una cartella in Documenti), salva le impostazioni prima di cambiarle e <b>non chiude mai Word, Excel o Outlook mentre ci lavori</b>: ti chiede di salvare e chiuderli tu (l\'unica eccezione è Outlook bloccato da più di 2 minuti senza finestra). I pulsanti <b>Controlla</b> (invece di Risolvi) sono i casi che non si possono sistemare da soli: Tasso trova la causa e ti spiega cosa fare.')]
S += [table([['Problema', 'Cosa fa Tasso'],
             ['<b><font color="#2F6BE0">STAMPA E SCANNER</font></b>', ''],
             ['<b>La stampante dice "offline"</b>', 'Riavvia il servizio di stampa, toglie la spunta "Usa stampante offline" e controlla se le stampanti di rete rispondono.'],
             ['<b>Stampa sempre sulla stampante sbagliata</b>', 'Spegne "Lascia che Windows gestisca la stampante predefinita" (che la cambia da solo) e ti fa scegliere quella giusta una volta per tutte.'],
             ['<b>Manca "Stampa su PDF"</b>', 'Rimette la stampante "Microsoft Print to PDF", quella che salva qualsiasi documento come PDF.'],
             ['<b>Non trovo la stampante (o i PC) in rete</b>', 'Accende i servizi di Windows che cercano stampanti e PC nella rete dell\'ufficio. Se la rete è "Pubblica" ti spiega perché sono nascosti.'],
             ['<b>La stampante USB non si vede</b>', 'Fa ricercare a Windows le stampanti collegate col cavo, riavvia quelle bloccate e il servizio di stampa.'],
             ['<b>Lo scanner non funziona</b>', 'Riavvia il servizio di Windows per scanner e fotocamere, sblocca gli scanner bloccati e ti apre il programma per scansionare.'],
             ['<b><font color="#2F6BE0">WORD, EXCEL, OFFICE E ADOBE</font></b>', ''],
             ['<b>Office si blocca o dà errori</b>', 'Fa la "riparazione rapida" ufficiale di Microsoft Office, senza internet. Documenti e impostazioni restano. Office deve essere chiuso.'],
             ['<b>Aggiorna Office</b>', 'Scarica e installa gli aggiornamenti ufficiali di Word, Excel, Outlook e PowerPoint. Molti errori spariscono così.'],
             ['<b>Ho perso un documento non salvato</b>', 'Cerca le copie automatiche di Word, Excel e PowerPoint degli ultimi 14 giorni e te le mette in una cartella in Documenti. Non cancella niente.'],
             ['<b>Word o Excel si chiudono appena li apri</b>', 'Mette da parte il modello di Word rovinato (Normal.dotm, Word ne crea uno nuovo) e ti dice quali componenti aggiuntivi partono con Office.'],
             ['<b>Office chiede sempre di accedere o dice "senza licenza"</b>', 'Svuota l\'accesso e le password salvate di Office (il registro con copia di sicurezza): poi rientri una volta con il tuo account e torna tutto a posto.'],
             ['<b>Manca il pulsante di Teams in Outlook</b>', 'Riaccende il componente "Riunione di Teams" di Outlook quando sparisce e svuota la cache dei componenti aggiuntivi di Office.'],
             ['<b>Word o Excel lenti, sfarfallano o restano bianchi</b>', 'Spegne l\'accelerazione grafica di Office, la causa più comune di finestre bianche, sfarfallii e scatti mentre scorri. Puoi tornare indietro.'],
             ['<b>Word o Excel non salvano su OneDrive o SharePoint</b>', 'Mette da parte la memoria dei documenti di Office (si rovina e blocca i salvataggi online, "Caricamento in sospeso"). Office ne crea una nuova. Non cancella niente.'],
             ['<b>Il file scaricato si apre in "Visualizzazione protetta"</b>', 'Ti spiega perché Word ed Excel aprono i file arrivati da internet in sola lettura e come sbloccare solo quelli di cui ti fidi. La protezione non la spegne: serve contro i virus nelle email. Controlla e spiega.'],
             ['<b>Il file è "bloccato da un altro utente"</b>', 'Chiude le copie di Word, Excel e PowerPoint rimaste aperte in sottofondo senza finestra (spesso l\'"altro utente" sei tu) e ti spiega cosa fare se il file è aperto davvero da un collega.'],
             ['<b>Date, decimali o file CSV sbagliati in Excel</b>', 'Rimette i formati italiani di Windows che usa Excel: date giorno/mese/anno, virgola per i decimali, punto e virgola nei file CSV (così non finisce tutto in una colonna). Puoi tornare indietro.'],
             ['<b>Adobe Acrobat Reader non apre i PDF o si blocca</b>', 'Chiude Acrobat se è bloccato, mette da parte le sue impostazioni rovinate (riparte come nuovo) e avvia la riparazione ufficiale di Adobe. I tuoi PDF non si toccano.'],
             ['<b><font color="#2F6BE0">OUTLOOK E POSTA</font></b>', ''],
             ['<b>Outlook non si apre</b>', 'Se Outlook è bloccato da più di 2 minuti senza finestra lo chiude, poi lo riapre sistemando il riquadro delle cartelle (la causa più comune). Posta e cartelle restano.'],
             ['<b>La ricerca di Outlook non trova le email</b>', 'Fa ricostruire a Windows l\'indice di ricerca da zero. Ci vogliono alcune ore col PC acceso, poi la ricerca torna completa.'],
             ['<b>Le email non arrivano o non partono</b>', 'Controlla internet, i server di posta Microsoft e lo spazio su disco e ti dice dov\'è il problema. Le impostazioni dell\'account le cambi tu.'],
             ['<b>Outlook lento: la casella è troppo grande</b>', 'Misura i file della posta di Outlook e ti dice se sono troppo grandi e cosa fare per alleggerirli. Non cancella email.'],
             ['<b>Outlook chiede sempre la password</b>', 'Toglie le password vecchie di Outlook e Office salvate in Windows: le reinserisci una volta e restano.'],
             ['<b>Gli allegati di Outlook non si aprono</b>', 'Svuota la cartella nascosta dove Outlook apre gli allegati, quando è piena e dà errore. I file vengono spostati in Documenti, non cancellati.'],
             ['<b>I promemoria di Outlook impazziscono</b>', 'Ricrea l\'elenco dei promemoria quando ne compaiono di vecchi o doppi, o non compaiono per niente. Appuntamenti e attività restano.'],
             ['<b>Outlook non si aggiorna o mostra cartelle vuote</b>', 'Mette da parte il file locale di Outlook (.ost): Outlook lo riscarica dal server con le mail giuste. Solo per account Microsoft 365, Exchange, Outlook.com e IMAP, dove le mail sono online. Non cancella niente.'],
             ['<b>Outlook è cambiato: rivoglio quello di prima</b>', 'Se Windows è passato da solo al "nuovo Outlook", rimette Outlook classico come predefinito (se è installato) e lo apre. Le mail sono le stesse: cambia solo il programma.'],
             ['<b>I link nelle email non si aprono</b>', 'Controlla il browser predefinito, il problema più comune quando Outlook dice "criteri dell\'organizzazione" o non fa niente. Ti apre la pagina giusta per sceglierlo di nuovo (Windows non lascia farlo ai programmi). Controlla e spiega.'],
             ['<b>"Nessun programma di posta" quando invio un file per email</b>', 'Rimette Outlook come programma di posta di Windows, quello che usano "Invia a > Destinatario" e i pulsanti "Invia per email" degli altri programmi. Puoi tornare indietro.'],
             ['<b><font color="#2F6BE0">TEAMS E RIUNIONI</font></b>', ''],
             ['<b>Teams lento, bianco o non si apre</b>', 'Chiude Teams e svuota la sua cache, la soluzione ufficiale di Microsoft. Chat, file e riunioni restano (sono online). Se sei in riunione, prima esci: Teams si chiude.'],
             ['<b>In riunione non mi sentono</b>', 'Toglie il muto al microfono e alza il volume se è a zero, controlla che Windows non blocchi il microfono a Teams, Zoom e Meet e, solo se non lo vede, riavvia l\'audio.'],
             ['<b>La webcam non va</b>', 'Ti dice se la webcam è già usata da un altro programma, la riaccende se era spenta, riavvia il servizio della fotocamera e controlla la privacy.'],
             ['<b>Le cuffie Bluetooth non si collegano</b>', 'Riavvia il Bluetooth e i suoi servizi audio. Ti spiega anche perché in chiamata le cuffie Bluetooth si sentono peggio.'],
             ['<b>Non sento gli altri in riunione</b>', 'Toglie il muto alle casse e alza il volume se è a zero, riavvia l\'audio solo se Windows non le vede e ti dice dove scegliere le cuffie giuste.'],
             ['<b>Non arrivano le notifiche di Teams e Outlook</b>', 'Riaccende le notifiche di Windows e quelle di Teams e Outlook se erano spente, e ti dice come vedere se è acceso "Non disturbare". Puoi tornare indietro.'],
             ['<b>La webcam è "usata da un altro programma"</b>', 'Ti dice quale programma sta usando adesso la webcam o il microfono (Teams, Zoom, il browser...), così sai cosa chiudere. Controlla e spiega.'],
             ['<b>Il volume si abbassa da solo durante le chiamate</b>', 'Dice a Windows di non abbassare più musica e altri suoni quando sei in chiamata su Teams, Zoom o Skype. Puoi tornare indietro.'],
             ['<b><font color="#2F6BE0">FILE, CARTELLE E ZIP</font></b>', ''],
             ['<b>I file si aprono con il programma sbagliato</b>', 'Ti dice con quale programma si aprono PDF, documenti di Word ed Excel, foto, file zip e link, e ti apre la pagina giusta per cambiarlo (Windows non lascia farlo ai programmi, per sicurezza). Controlla e spiega.'],
             ['<b>I file .zip non si aprono o danno errore</b>', 'Controlla con cosa si aprono i file zip, rimette le "cartelle compresse" di Windows se un programma disinstallato le ha rotte e ti dice cosa fare con zip protetti da password, file bloccati e nomi troppo lunghi.'],
             ['<b>"Il nome del file è troppo lungo"</b>', 'Accende in Windows i percorsi lunghi (oltre 260 caratteri), utile con le cartelle di OneDrive, SharePoint e del server. Ti dice anche come accorciare, perché alcuni programmi restano al limite.'],
             ['<b>Non posso cancellare o rinominare un file ("è aperto in un altro programma")</b>', 'Chiude l\'anteprima di Esplora file e riavvia Esplora file, che spesso tengono il file occupato. Le finestre delle cartelle si chiudono: se stai copiando file, aspetta che finisca.'],
             ['<b>Ho cancellato un file per sbaglio</b>', 'Ti dice dove può essere: Cestino, Cestino di OneDrive, versioni precedenti, Cronologia file. Apre il Cestino. Se non c\'è, prova lo strumento "Recupera file cancellati" di Tasso. Controlla e spiega.'],
             ['<b>Una cartella o un file è sparito (forse è nascosto)</b>', 'Fa vedere file e cartelle nascosti e, se una chiavetta ha le cartelle nascoste da un virus (succede spesso), le fa ricomparire. Puoi tornare indietro.'],
             ['<b>Icone del desktop che non aprono più niente</b>', 'Trova sul desktop i collegamenti che puntano a programmi o file che non ci sono più e li sposta in una cartella dentro Documenti (non li cancella).'],
             ['<b>Le cartelle si aprono lente o con la vista sbagliata</b>', 'Azzera le viste salvate delle cartelle (icone, colonne, ordine), che col tempo si rovinano e rallentano Esplora file. Con copia di sicurezza. Cartelle e file non si toccano.'],
             ['<b>Esplora file si apre su "Home" invece di "Questo PC"</b>', 'Fa aprire Esplora file direttamente su Questo PC, con dischi, chiavette e unità di rete in vista. Puoi tornare indietro.'],
             ['<b>Il disco C: è pieno</b>', 'Ti dice cosa occupa più spazio (Download, Desktop, Documenti, Cestino, OneDrive, vecchio Windows) e cosa puoi liberare in sicurezza. Non cancella niente da solo. Controlla e spiega.'],
             ['<b>Windows 11: il menu del tasto destro è corto</b>', 'Rimette il menu completo del tasto destro (come in Windows 10), senza dover premere "Mostra altre opzioni" ogni volta. Puoi tornare indietro.'],
             ['<b><font color="#2F6BE0">RETE, WI-FI E VPN</font></b>', ''],
             ['<b>Il Wi-Fi si stacca spesso</b>', 'Impedisce a Windows di spegnere il Wi-Fi per risparmiare energia (la causa più comune) e ti dice quanto è forte il segnale.'],
             ['<b>Il Wi-Fi non si collega</b>', 'Riaccende la scheda Wi-Fi se è spenta, riavvia il servizio del Wi-Fi e la scheda. Le reti e le password salvate restano.'],
             ['<b>La VPN non si collega</b>', 'Riavvia i servizi di Windows per le VPN e quelli del programma VPN dell\'ufficio (FortiClient, GlobalProtect, Cisco...), e rimette l\'ora giusta.'],
             ['<b>Con la VPN i siti dell\'ufficio non si aprono</b>', 'Svuota la memoria degli indirizzi (DNS), registra di nuovo il PC nella rete e ti mostra quali server DNS sta usando.'],
             ['<b>Le unità di rete hanno la X rossa</b>', 'Controlla le unità di rete (Z:, S:...) una per una, ti dice quali server rispondono e apre quelle raggiungibili per ricollegarle.'],
             ['<b>La cartella di rete chiede sempre la password</b>', 'Ti mostra le password salvate per i server dell\'ufficio e ti apre il posto dove sistemarle. La password nuova la scrivi tu.'],
             ['<b>Internet c\'è ma i siti non si aprono</b>', 'Controlla il "proxy" (un passaggio obbligato per internet): se è rimasto quello di un programma tolto e non funziona, lo spegne (con copia di sicurezza).'],
             ['<b>Il cavo di rete è collegato ma non va</b>', 'Controlla la scheda di rete del cavo: se è spenta la riaccende, se va lenta ti dice di cambiare cavo, poi la riavvia e chiede di nuovo l\'indirizzo al router.'],
             ['<b>Aggiornamenti e OneDrive fermi: "connessione a consumo"</b>', 'Se la rete Wi-Fi che usi è segnata "a consumo", Windows e OneDrive non scaricano. Toglie il segno dalla rete di casa o dell\'ufficio (non dall\'hotspot del telefono). Puoi rimetterlo.'],
             ['<b>Internet è lento solo su questo PC</b>', 'Controlla segnale Wi-Fi, velocità del cavo, VPN e i programmi che scaricano o caricano in sottofondo (OneDrive, aggiornamenti). Ti dice cosa rallenta. Controlla e spiega.'],
             ['<b>Desktop remoto: schermo nero, lento o non si collega</b>', 'Svuota la memoria delle immagini del Desktop remoto (causa schermi neri e pezzi di finestra sbagliati) e controlla se i computer a cui ti colleghi rispondono. Le connessioni salvate restano.'],
             ['<b>Un sito di lavoro è bloccato senza motivo</b>', 'Controlla il file "hosts" di Windows, dove virus o vecchi programmi possono bloccare o deviare siti come Microsoft, Google o la banca. Mette da parte solo le righe sospette, con copia di sicurezza.'],
             ['<b>Il portatile non va su internet in un\'altra rete</b>', 'Se la scheda di rete ha un indirizzo o un DNS messi a mano (di un\'altra rete) e qui non funzionano, li rimette automatici. Salva prima i valori vecchi in un file, per rimetterli in ufficio.'],
             ['<b><font color="#2F6BE0">SCHERMI E USB</font></b>', ''],
             ['<b>Il secondo schermo è nero</b>', 'Fa ricercare gli schermi collegati e mette "Estendi" (come Win+P), così il secondo monitor si accende accanto al primo.'],
             ['<b>Il proiettore o la TV non mostrano niente</b>', 'Mette "Duplica" (come Win+P): il proiettore o la TV mostrano lo stesso schermo del portatile.'],
             ['<b>Lo schermo è girato</b>', 'Rimette dritti gli schermi girati di lato o sottosopra (succede premendo per sbaglio Ctrl+Alt+frecce).'],
             ['<b>Scritte sfocate o troppo piccole</b>', 'Accende la correzione di Windows per i programmi sfocati e la scrittura più nitida (ClearType). Poi ti apre la grandezza del testo.'],
             ['<b>Chiavetta, disco USB o docking non vanno</b>', 'Riavvia le porte USB bloccate, fa ricercare i dispositivi e dà una lettera (E:, F:...) alle chiavette che non ce l\'hanno. Non formatta mai niente.'],
             ['<b>Mouse, chiavette o periferiche USB si staccano da sole</b>', 'Spegne il risparmio energetico delle porte USB, la causa più comune di mouse, tastiere, chiavette e docking che si staccano. Consuma pochissimo in più. Puoi tornare indietro.'],
             ['<b>Lo schermo si spegne o si blocca troppo presto</b>', 'Se lo schermo si spegne o chiede la password dopo 1-2 minuti, porta l\'attesa a 10 minuti con la corrente e 5 a batteria. Ti dice anche se è l\'ufficio a deciderlo.'],
             ['<b>La luminosità cambia da sola</b>', 'Spegne la luminosità automatica di Windows (quella che scurisce lo schermo con il sensore di luce) e ti dice cosa altro la cambia. Puoi riaccenderla dalle impostazioni.'],
             ['<b>Win + Maiusc + S (cattura dello schermo) non funziona</b>', 'Controlla che lo Strumento di cattura ci sia e funzioni, fa catturare lo schermo anche con il tasto Stamp e ti dice dove finiscono le catture. Puoi tornare indietro.'],
             ['<b><font color="#2F6BE0">MOUSE, TASTIERA E PORTATILE</font></b>', ''],
             ['<b>La tastiera scrive i simboli sbagliati</b>', 'Rimette la tastiera italiana come predefinita e spegne Alt+Maiusc, la scorciatoia che la cambia per sbaglio in inglese (con copia di sicurezza).'],
             ['<b>Copia e incolla non funziona</b>', 'Riavvia il servizio degli appunti di Windows (e quello del Desktop remoto, se lo usi).'],
             ['<b>Voglio ritrovare le cose copiate prima (Win + V)</b>', 'Accende la cronologia degli appunti: con Win + V ritrovi le ultime cose copiate, testi e immagini. Resta solo su questo PC. Puoi spegnerla quando vuoi.'],
             ['<b>Il mouse va troppo piano, troppo veloce o il doppio clic non va</b>', 'Rimette velocità del puntatore, doppio clic e rotellina come quando Windows è nuovo (con copia di sicurezza) e ti dice se i tasti del mouse sono invertiti.'],
             ['<b>Il touchpad del portatile non funziona</b>', 'Riaccende il touchpad se era spento in Gestione dispositivi, riavvia il suo driver e ti dice dove si accende in Windows e quale tasto Fn lo spegne.'],
             ['<b>Il tastierino numerico è spento quando accendo il PC</b>', 'Fa partire Windows con il Bloc Num acceso, così il PIN e i numeri a destra della tastiera funzionano subito. Puoi tornare indietro.'],
             ['<b>La tastiera fa bip, scrive lento o i tasti restano premuti</b>', 'Spegne Tasti permanenti, Filtro tasti e Segnali acustici (si accendono per sbaglio premendo Maiusc 5 volte o tenendolo premuto) e le loro scorciatoie. Con copia di sicurezza.'],
             ['<b>Il tasto Windows non funziona</b>', 'Controlla se il tasto Windows è bloccato da un\'impostazione e lo sblocca (se non l\'ha deciso l\'ufficio), con copia di sicurezza. Ti dice anche il blocco "Gaming" che hanno molte tastiere.'],
             ['<b>La batteria del portatile dura poco</b>', 'Legge quanto si è consumata la batteria rispetto a nuova e i cicli di ricarica, e ti dice cosa la scarica e se è da cambiare. Controlla e spiega.'],
             ['<b><font color="#2F6BE0">WINDOWS LENTO O BLOCCATO</font></b>', ''],
             ['<b>Esplora file si blocca o è lento</b>', 'Riavvia Esplora file e controlla se un\'unità di rete che non risponde lo sta rallentando. Le finestre delle cartelle aperte si chiudono: se stai copiando file, aspetta che finisca.'],
             ['<b>Barra in basso bloccata o icone sparite</b>', 'Ricrea le icone di Windows e riavvia barra delle applicazioni, menu Start e area notifiche. Se stai copiando file, aspetta che finisca.'],
             ['<b>La barra in basso si nasconde da sola</b>', 'Spegne "Nascondi automaticamente la barra delle applicazioni", così la barra con Start e i programmi aperti resta sempre visibile. Puoi tornare indietro.'],
             ['<b>Le icone del desktop sono sparite</b>', 'Rifà vedere le icone del desktop se erano nascoste, rimette il Cestino (e Questo PC, se era stato tolto) e ti dice se il desktop è finito dentro OneDrive. Puoi tornare indietro.'],
             ['<b>Un programma non risponde</b>', 'Trova i programmi bloccati ("Non risponde") da più di 20 secondi e li chiude. Word, Excel e Outlook non li chiude mai (spesso stanno solo salvando). Il lavoro non salvato nel programma chiuso si perde.'],
             ['<b>Il PC è lento adesso</b>', 'Guarda cosa stanno usando processore, memoria e disco in questo momento e ti dice quale programma pesa di più e cosa fare. Non chiude niente. Controlla e spiega.'],
             ['<b>Scritte strane o caratteri che mancano</b>', 'Ricostruisce la memoria dei caratteri di Windows quando le scritte si vedono a quadratini o con il carattere sbagliato. Serve un riavvio.'],
             ['<b>Non vedo le anteprime di foto e documenti</b>', 'Riaccende le anteprime in Esplora file se erano spente e ricrea le miniature rovinate. Se stai copiando file, aspetta che finisca.'],
             ['<b>Non vedo .pdf .docx alla fine dei file</b>', 'Fa vedere sempre l\'estensione dei file (.pdf, .docx, .exe). Così capisci cos\'è un file e riconosci gli allegati truffa come fattura.pdf.exe.'],
             ['<b>Il PC ci mette tanto ad accendersi</b>', 'Misura quanto ci mette Windows a partire, trova i programmi che lo rallentano e ti dice cosa spegnere. Controlla e spiega.'],
             ['<b>Spegnendo il PC i problemi non spariscono</b>', 'Spegne l\'"Avvio rapido": così "Arresta il sistema" spegne davvero Windows e ripulisce tutto come un riavvio. L\'accensione può durare qualche secondo in più. Puoi tornare indietro.'],
             ['<b>Il PC non va in sospensione o si riaccende da solo</b>', 'Ti dice quale programma o dispositivo tiene sveglio il PC o lo ha riacceso. Controlla e spiega cosa fare.'],
             ['<b><font color="#2F6BE0">ORA, ONEDRIVE, ACCOUNT E SICUREZZA</font></b>', ''],
             ['<b>L\'ora è sbagliata (e i siti danno errore)</b>', 'Rimette l\'ora giusta da internet. Con l\'ora sbagliata i siti dicono "connessione non privata" e VPN e Office non si collegano.'],
             ['<b>OneDrive non sincronizza</b>', 'Fa il "reset" ufficiale di OneDrive: si riavvia e ricontrolla tutti i file. I file non vengono cancellati, né sul PC né online.'],
             ['<b>Libera spazio con OneDrive</b>', 'I file di OneDrive che non modifichi da 60 giorni restano online e smettono di occupare il disco. Li vedi sempre, si riscaricano quando li apri (serve internet).'],
             ['<b>In OneDrive ci sono file doppi con il nome del PC</b>', 'Trova le copie che OneDrive crea quando lo stesso file viene cambiato in due posti (es. "Offerta-PCUFFICIO.docx") e te le elenca in un file, così scegli tu quale tenere. Non cancella niente. Controlla e spiega.'],
             ['<b>Controllo dei programmi sospetti</b>', 'Fa la scansione veloce dell\'antivirus di Windows e cerca i programmi senza firma che partono da soli da cartelle insolite. Ti dice cosa guardare, non cancella niente da solo.'],
             ['<b>Compare la scritta "Attiva Windows"</b>', 'Controlla se Windows è attivato e con che licenza, e ti spiega cosa fare. Tasso non usa e non userà mai programmi pirata per attivare. Controlla e spiega.'],
             ['<b>Sono entrato con un "profilo temporaneo" (desktop vuoto)</b>', 'Controlla se Windows ti ha fatto entrare con un profilo temporaneo e ti dice dov\'è il tuo profilo vero: i tuoi file di solito sono ancora tutti lì. Controlla e spiega.'],
             ['<b>La password di Windows sta per scadere</b>', 'Ti dice quando scade la password del tuo account di Windows o dell\'ufficio e come cambiarla prima (Ctrl + Alt + Canc > Cambia password). Tasso non vede né salva la password. Controlla e spiega.'],
             ['<b>Il PC chiede la chiave di ripristino BitLocker</b>', 'Controlla se i dischi sono protetti con BitLocker e ti dice dove trovare la chiave di ripristino (account Microsoft o dell\'ufficio) prima che serva. Tasso non mostra né salva la chiave. Controlla e spiega.']],
            [56 * mm, W - 2 * M - 56 * mm])]
S += [P('<b>Driver di tutto il PC</b>', 'h2'),
      P('Il <b>driver</b> è il "libretto di istruzioni" che dice a Windows come usare un pezzo del PC. In <b>Risolvi problemi</b> Tasso elenca tutte le periferiche (scheda video AMD Radeon, NVIDIA o Intel, audio, cuffie, mouse, tastiera, rete, Bluetooth, webcam, monitor...), segna quelle con problemi con il pulsante <b>Ripara</b>, cerca i driver nuovi su <b>Windows Update</b> (certificati da Microsoft) e li installa. Per scheda video e periferiche da gaming apre il <b>sito ufficiale</b> della marca (NVIDIA App, AMD, Intel, Logitech, Razer, Corsair, SteelSeries, scheda madre). L\'interruttore <b>Windows scarica da solo i driver</b> resta anche dopo il riavvio. Mai siti di "driver gratis" o programmi che "aggiornano tutti i driver".')]

# 8 Internet, disco e giochi (2.1)
S += chapter(8, 'Internet, disco e giochi', 'novità dalla 2.1 alla 2.5')
S += bullets(['<b>Salute del PC</b> (in Home): un voto da 0 a 100 con i problemi più importanti e un pulsante per sistemarli. Controlla spazio libero, programmi all\'avvio, file temporanei, Cestino, antivirus, aggiornamenti di Windows, riavvii in sospeso, memoria e salute dei dischi.',
              '<b>Internet, test di velocità</b>: download, upload e ping in circa 30 secondi, con un giudizio semplice (va bene per film, giochi online, download).',
              '<b>Internet, la tua connessione</b>: come sei collegato (Wi-Fi o cavo) e consigli utili, per esempio "collegati alla rete a 5 GHz" quando il tuo router ce l\'ha, o "usa il cavo" se il segnale è debole. Non mostra più i dati consumati.',
              '<b>Internet, Wi-Fi salvati</b>: le reti a cui il PC si è collegato e la loro password, nascosta finché non premi Mostra. Comodo per darla a un ospite.',
              '<b>Mappa del disco</b>: le cartelle che occupano più spazio, dalla più grande, con il pulsante per aprirle. Tasso guarda e basta: non cancella niente.',
              '<b>Modalità gioco automatica</b> (Impostazioni): quando apri un gioco di Steam, Epic, Riot, Xbox e simili, Tasso passa da solo alla modalità Gaming; un minuto dopo che lo hai chiuso rimette la modalità di prima.'])

S += [P('<b>Giochi (novità della 2.2)</b>', 'h2')]
S += bullets(['<b>Ottimizza prima di giocare</b>: Tasso ti mostra i programmi aperti che usano più memoria. Scegli quali chiudere (i programmi con una finestra si chiudono come con la X, quindi ti chiedono di salvare se serve) e premi <b>Ottimizza e gioca</b>: passa anche alla modalità Gaming. A fine partita premi <b>Fine partita</b>: rimette la modalità di prima e riapre i programmi chiusi. Windows, driver, antivirus, Discord, i launcher dei giochi e i programmi per lo streaming (OBS, Streamlabs, Twitch Studio, Medal) non vengono mai toccati, e Tasso chiude solo quello che spunti tu. Se riduci il gioco a icona per andare un attimo su Chrome, Tasso lo considera ancora aperto e il pannellino si nasconde finché non torni nel gioco.',
              '<b>Pannellino in gioco</b>: una scritta piccola e trasparente in alto a destra o a sinistra con uso e temperatura di processore e scheda video, RAM e ora. Non si può cliccare. Appare da solo quando giochi (con la modalità gioco automatica accesa) e si mostra o nasconde con <b>Ctrl + Maiusc + F12</b>. Si vede nei giochi in finestra o "schermo intero senza bordi". La temperatura del processore appare se Windows la fornisce o se è aperto LibreHardwareMonitor.',
              '<b>FPS</b>: Tasso non entra mai dentro i giochi, perché con gli anti-cheat si rischia il ban. Per vedere gli FPS usa gli strumenti ufficiali: <b>Win + G</b> (Prestazioni) di Windows oppure <b>Alt + R</b> di NVIDIA.',
              '<b>Autoclick</b>: clicca da solo ogni tot millisecondi, secondi o minuti, per un numero di volte o all\'infinito, con il tasto sinistro, destro o centrale, singolo o doppio, dove si trova il mouse o in un punto fisso. Può fare piccoli movimenti del mouse che restano dentro la finestra del gioco. Si avvia e si ferma con un tasto che scegli tu (di base F6); <b>Esc</b> lo ferma sempre.',
              '<b>La tua libreria</b>: tutti i giochi installati con Steam, Epic Games e Riot in una pagina sola, con la ricerca e il pulsante <b>Gioca</b>.'])
S += [box('Autoclick e giochi online', ['Nei giochi online con anti-cheat (League of Legends, Valorant, Fortnite, CS2 e altri) l\'autoclick è contro le regole e può portare al ban dell\'account. Usalo nei giochi per un giocatore, nei giochi "clicker" o dove è permesso.'], AMBER, AMBER_BG)]

S += [P('<b>Novità della 2.6</b>', 'h2')]
S += bullets(['<b>Rileva i problemi</b> (in cima al menu, subito sotto Home insieme a Risolvi problemi, e pulsante in Home): un clic e Tasso controlla 58 problemi diversi (54 in modalità Ufficio) in circa un minuto. <b>Non cambia niente</b>: ti mostra solo quelli che ha trovato davvero, ognuno con il suo pulsante Risolvi, e "Risolvi insieme" per sistemarli tutti in una volta. Alcuni problemi si vedono solo quando succedono (per esempio il secondo schermo nero): per quelli c\'è Risolvi problemi.',
              '<b>Internet</b>: al posto del consumo di dati c\'è <b>La tua connessione</b>, con consigli utili (per esempio "collegati alla rete a 5 GHz" quando il tuo router ce l\'ha).',
              '<b>Due modalità</b>: Gaming e Ufficio, scelte alla prima apertura e cambiabili quando vuoi dal pulsante in alto a sinistra. In Ufficio Tasso è più semplice e tranquillo (colori chiari e morbidi, un tasso più carino) e nasconde giochi, FPS, ping, Turbo e Prestazioni+.',
              '<b>I 100 problemi più comuni dell\'ufficio</b> in Risolvi problemi (stampa, Word, Excel, Adobe, Outlook, Teams, file e zip, rete, schermi, mouse e tastiera, account), ognuno con un pulsante e una spiegazione semplice (vedi capitolo 7).',
              '<b>Risolvi problemi diviso in sezioni</b> (Ufficio, Gaming, Windows) con <b>una ricerca sola</b> per tutti i problemi, e <b>3 problemi da gioco nuovi</b>: mira del mouse, gioco che si riduce a icona, download lenti.',
              '<b>Voto al PC da 1 a 100</b> per tutto, con il voto totale e la classifica (da "PC lento" a "top").',
              '<b>Giochi</b>: la tua libreria è in cima alla pagina. <b>Prestazioni+</b> riordinata: prima le cose facili e sicure, poi quelle per esperti.',
              '<b>6 temi nuovi</b> per l\'ufficio: Ufficio, Carta, Salvia, Lavanda, Notte e Ardesia (8 in tutto, con Ghiaccio e Sabbia). Gaming e Ufficio ricordano ognuna il suo tema.'])
S += [P('<b>Novità della 2.5</b>', 'h2')]
S += bullets(['<b>Profilo gioco</b> (in Giochi): scegli una volta cosa ti serve per giocare (Gaming o FPS competitivo, i comandi gaming, il pannellino, la modalità gioco automatica) e lo accendi con <b>un interruttore</b>. <b>Resta acceso anche se spegni o riavvii il PC</b>: a ogni accensione Tasso controlla che sia tutto come l\'hai scelto e rimette quello che è cambiato (per esempio il piano energetico rimesso normale da un aggiornamento). Quando lo spegni torna tutto com\'era prima.',
              '<b>Potenza per ogni gioco</b>: scegli un gioco e <b>Potenza alta</b> (modalità Gaming) o <b>Massima potenza</b> (FPS competitivo, con il mouse senza accelerazione). Quando apri quel gioco Tasso la mette da solo, quando lo chiudi rimette com\'era.',
              '<b>Ping ai server dei giochi</b> (in Internet): League of Legends, TFT, Valorant, CS2, Dota 2, Fortnite e altri giochi sui server Amazon. Vedi i millisecondi, i salti e i pacchetti persi, e Tasso ti dice qual è il server migliore per te.',
              '<b>Regola con Tasso</b> (sotto ogni gioco): capisce se il gioco è competitivo, grafico o "prestazionale", guarda il tuo PC e propone la grafica giusta. Applica da solo quello che può in sicurezza, con <b>Annulla</b>.',
              '<b>Comandi gaming</b> spiegati in parole semplici: per ognuno <b>cosa fa</b>, <b>quando serve</b> e <b>se ci sono rischi</b>. Restano accesi dopo il riavvio; spenti tornano esattamente com\'erano prima di Tasso.',
              '<b>Aggiornamento programmi</b> (in Programmi): l\'elenco dei programmi con una versione nuova, da aggiornare uno per uno o tutti insieme, e le <b>icone vere</b> dei programmi.',
              '<b>Portachiavi</b> (nel menu a sinistra): le tue password cifrate (AES-256) e salvate solo su questo PC. Scegli un <b>PIN da 4 numeri</b> (comodo ma più debole), <b>da 6</b> (consigliato), <b>da 7 a 12</b> (il più sicuro) oppure <b>senza PIN</b>: si apre da solo con il tuo account di Windows, ma chi usa il PC con il tuo account vede le password. Genera password forti e controlla se sono state rubate (al servizio Have I Been Pwned arriva solo un pezzetto dell\'impronta, mai la password).',
              '<b>Stato del PC su Discord</b> più ricco, ogni 3/6/12/24 ore e su richiesta con !stato.'])
S += [box('Se scordi il PIN del portachiavi', ['Nessuno può recuperarlo, nemmeno Tasso: è proprio questo che lo rende sicuro. Puoi solo azzerare il portachiavi e ricominciare da capo.'], AMBER, AMBER_BG)]

# 9 Hardware
S += chapter(9, 'Hardware', 'conosci il tuo PC')
S += bullets(['<b>Componenti</b>: processore, scheda video, scheda madre e BIOS, RAM, dischi, versione di Windows.',
              '<b>Sensori in tempo reale</b>: uso del processore e della RAM, spazio del disco; con le schede <b>NVIDIA</b> anche temperatura, uso, ventola e memoria video; temperatura dei dischi quando Windows la rende disponibile.',
              '<b>Scheda video</b>: il pulsante apre il pannello NVIDIA o AMD, oppure la pagina ufficiale per scaricarlo.'])
S += [box('Il BIOS: aggiornarlo sì o no?', ['Tasso ti mostra la versione del BIOS e cerca la pagina ufficiale del produttore della scheda madre, ma <b>non lo aggiorna da solo</b>. Aggiornalo solo se risolve un problema che hai o se il produttore lo consiglia: se il PC si spegne a metà aggiornamento, la scheda madre può bloccarsi. Segui sempre le istruzioni del produttore, con il PC collegato alla corrente.'], AMBER, AMBER_BG)]

# 9 Impostazioni
S += chapter(10, 'Impostazioni', 'su misura')
S += [table([['Impostazione', 'Cosa fa'],
             ['<b>Aspetto (8 temi)</b>', 'Tasso, Aurora, Lava, Neon, Carbonio, Belva (scuri) e Ghiaccio, Sabbia (chiari). Tocca una scheda e l\'app cambia colore subito.'],
             ['<b>Tasso acceso</b>', 'L\'interruttore generale: vedi il capitolo 11.'],
             ['<b>Avvia con Windows</b>', 'Tasso parte da solo, ridotto a icona, quando accendi il PC.'],
             ['<b>Avvisi intelligenti</b>', 'Piccoli avvisi in basso a destra (disco quasi pieno, programma nuovo all\'avvio). Mai in modalità Gaming o FPS.'],
             ['<b>Modalità gioco automatica</b>', 'Quando apri un gioco Tasso passa alla modalità Gaming e, quando lo chiudi, rimette quella di prima.'],
             ['<b>Stato del PC su Discord</b>', 'Tasso scrive in un tuo canale privato di Discord il report del PC: voto di salute, processore, scheda video, scheda madre, RAM, dischi, temperature e internet, con le spunte verdi per quello che va bene. Scegli ogni quanto (3, 6, 12 o 24 ore); arriva anche dopo ogni pulizia. Serve l\'indirizzo di un webhook (Modifica canale, Integrazioni, Webhook, Copia URL), da incollare qui: la guida passo passo è dentro Tasso. Facoltativo: con un piccolo bot gratuito (guida in Tasso) scrivi <b>!stato</b> nel canale e il report arriva subito. Da Discord si può solo leggere: non si comanda il PC.'],
             ['<b>Scrivici</b>', 'Mandi un\'idea, un problema o un commento, con fino a 2 foto e, se vuoi, i dati del PC e la tua email per la risposta. C\'è anche in Info. Se internet non va, Tasso prepara l\'email già scritta.'],
             ['<b>Aiuta a migliorare Tasso</b>', 'Numeri anonimi (versione, Windows, marca di processore e scheda video, RAM, spazio liberato, pulsanti usati, errori). Mai nome, file o indirizzo IP. Tasso lo chiede una volta; qui lo accendi o lo spegni quando vuoi.'],
             ['<b>Modalità di Tasso</b>', 'Gaming o Ufficio, come il pulsante in alto a sinistra.'],
             ['<b>Torna indietro</b>', 'L\'elenco di tutto quello che Tasso ha cambiato (impostazioni di Windows, programmi all\'avvio, comandi gaming, giochi regolati, profilo gioco), con un pulsante per rimettere com\'era ogni cosa o tutto insieme (vedi capitolo 11).']],
            [42 * mm, W - 2 * M - 42 * mm])]

# 10 Spegnere
S += chapter(11, 'Spegnere Tasso e rimettere com\'era', 'tu decidi')
S += [P('In basso a sinistra c\'è il pulsante <b>"Tasso è acceso"</b>. Cliccalo per spegnerlo: da spento Tasso <b>non parte con Windows, non fa pulizie automatiche e non manda avvisi</b>. Quando lo spegni scegli cosa fare con le impostazioni che ha cambiato:')]
S += [table([['Scelta', 'Cosa succede'],
             ['<b>Spegni e rimetti com\'era prima</b>', 'Le impostazioni di Windows cambiate da Tasso tornano esattamente come erano prima che lo usassi.'],
             ['<b>Spegni e lascia tutto così</b>', 'Tasso si spegne ma le impostazioni restano come sono adesso.']],
            [56 * mm, W - 2 * M - 56 * mm])]
S += [box('Cosa torna com\'era e cosa no', ['<b>Torna com\'era:</b> piano energetico e limite del processore, spegnimento dello schermo e sospensione, notifiche, mouse, Modalità Gioco, impostazioni di privacy, programmi all\'avvio che hai acceso o spento da Tasso.',
                                            '<b>Non può tornare indietro:</b> i file cancellati, i programmi disinstallati e gli aggiornamenti installati. Per quelli c\'è il backup di Windows del capitolo 12.'], VIOLET, VIOLET_BG)]
S += [box('Spegnere o riavviare il PC non rimette niente', ['Tutto quello che Tasso cambia <b>resta così</b> anche se spegni o riavvii il PC. Torna com\'era solo quando lo decidi tu: dal pulsante qui sopra oppure da <b>Impostazioni → Torna indietro</b>, dove puoi rimettere com\'era una cosa alla volta o tutto insieme.'], GREEN, GREEN_BG)]
S += [P('Per riaccendere Tasso basta ricliccare il pulsante, oppure <b>Riaccendi Tasso</b> nella Home.', 'p')]

# 11 Sicurezza
S += chapter(12, 'Sicurezza e backup', 'niente paura')
S += bullets(['<b>Backup automatico</b>: prima delle pulizie Media, Profonda ed Eccezionale Tasso crea un <b>punto di ripristino</b> di Windows.',
              '<b>I tuoi file</b> non vengono mai cancellati senza conferma, e vanno nel Cestino.',
              '<b>Password e cookie</b> dei browser non vengono toccati: Tasso pulisce solo la cache.',
              '<b>Driver</b>: Tasso installa solo quelli certificati da Microsoft (Windows Update); per il resto apre il sito ufficiale del produttore.',
              '<b>Privacy</b>: lo strumento "Privacy: stop pubblicità e tracciamento" ha il suo tasto "Privacy: ripristina".'])
S += [P('<b>Come tornare indietro con il backup di Windows</b>', 'h2')]
S += steps(['Premi il tasto <b>Windows</b> e scrivi <b>Crea un punto di ripristino</b>, poi aprilo.',
            'Clicca <b>Ripristino configurazione di sistema</b> e poi <b>Avanti</b>.',
            'Scegli il punto chiamato <b>"Tasso - prima della manutenzione"</b> e conferma. Il PC si riavvia com\'era in quel momento (i tuoi documenti non vengono toccati).'])
S += [box('Cosa Tasso non fa, apposta', ['<b>Pulizia del registro</b>, <b>"libera RAM" continuo</b>, <b>acceleratori di internet</b> miracolosi, <b>driver da siti sconosciuti</b>, <b>overclock</b> e <b>aggiornamento automatico del BIOS</b>: o non servono davvero o sono troppo rischiosi da fare in automatico.'], RED, RED_BG)]

# 12 FAQ
S += chapter(13, 'Domande frequenti', 'dubbi veloci')
faq = [
    ('L\'antivirus o Windows lo bloccano: cosa faccio?', 'È l\'avviso per i programmi che Windows non conosce ancora (capitolo 1): clicca <b>Ulteriori informazioni</b> e <b>Esegui comunque</b>. Se un antivirus lo mette in quarantena, puoi segnalarlo come falso allarme al produttore dell\'antivirus; per Microsoft Defender si fa da microsoft.com/wdsi/filesubmission.'),
    ('Posso tenerlo su una chiavetta USB?', 'Sì. Copia tutta la cartella Tasso sulla chiavetta e avvia Tasso.exe da lì: non serve installarlo. Le impostazioni vengono salvate sul PC in cui lo usi.'),
    ('Come lo mando a un amico?', 'Mandagli il link fisso <b>github.com/sergiotassoni/tasso/raw/main/Tasso.zip</b>: scarica sempre l\'ultima versione, senza account. Oppure entra nel server Discord di Tasso, dove trovi il link nel canale #tasso.'),
    ('Rallenta il PC mentre gioco?', 'No. Tasso legge i sensori ogni 2 secondi solo mentre è aperto e lavora solo quando avvii qualcosa. In modalità Gaming e FPS non manda avvisi.'),
    ('Se spengo il PC le impostazioni tornano come prima?', 'No. Restano come le ha messe Tasso, anche il Profilo gioco. Tornano com\'erano solo se lo chiedi tu da <b>Impostazioni → Torna indietro</b>.'),
    ('Come si aggiorna Tasso?', 'Quando esce una versione nuova, in Home compare un avviso che ti porta in <b>Impostazioni → Aggiornamenti di Tasso</b>. Premi <b>Aggiorna ora</b>: Tasso scarica la nuova versione dalla pagina ufficiale, controlla che il file sia quello giusto e si riapre da solo. Le impostazioni restano. Se qualcosa non va, puoi sempre scaricare lo zip a mano.'),
    ('Dove salva le sue cose?', 'Nella cartella <b>%LOCALAPPDATA%\\Tasso</b>: impostazioni, registri di ogni pulizia e portachiavi cifrato.'),
    ('Una pulizia ci mette tanto: è normale?', 'Sì per Profonda ed Eccezionale: aggiornamenti di Windows, riparazione e deframmentazione possono richiedere tempo. Se un passaggio si blocca troppo a lungo, Tasso lo ferma da solo e va avanti.'),
    ('Si apre una finestra bianca o un messaggio su WebView2.', 'Manca il componente gratuito di Microsoft: premi OK, si apre la pagina ufficiale per installarlo, poi riapri Tasso.'),
]
for q, a in faq:
    S += [KeepTogether([P('<b>%s</b>' % q, 'p'), P(a, 'p'), Spacer(1, 2)])]
S += [Spacer(1, 6), box('In arrivo nelle prossime versioni', ['Firma digitale (serve un certificato a pagamento), lettore video e musica con sottotitoli, convertitori di file, registratore degli ultimi 30 secondi di gioco, statistiche delle ore giocate, blocco pubblicità, controllo webcam e microfono.'], BLUE, colors.HexColor('#EAF1FD'))]
S += [Spacer(1, 10), P('Tasso 3.0', 'small')]

doc.build(S)
print('ok')
