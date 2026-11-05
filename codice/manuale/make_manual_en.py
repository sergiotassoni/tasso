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
    return [CondPageBreak(90 * mm), Spacer(1, 6), P(('CHAPTER %d' % n) + ('  ·  ' + kicker if kicker else ''), 'kicker'), P(title, 'h1')]


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
    c.drawCentredString(W / 2, H / 2 - 48 * mm, "User manual")
    c.setFont('PopL', 11)
    c.drawCentredString(W / 2, H / 2 - 57 * mm, 'Cleans, repairs and boosts your PC. In one click.')
    c.setFont('Pop', 9)
    c.setFillColor(colors.HexColor('#8B91A7'))
    c.drawCentredString(W / 2, 18 * mm, 'Version 3.0  ·  Windows 10 and 11  ·  2026')
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
    c.drawString(M + 14 * mm, H - 10 * mm, "User manual")
    c.drawRightString(W - M, 10 * mm, str(doc.page))
    c.restoreState()


doc = BaseDocTemplate(OUT, pagesize=A4, leftMargin=M, rightMargin=M, topMargin=18 * mm, bottomMargin=17 * mm,
                      title="Tasso - User manual", author='Tasso', subject='Tasso 2.6 manual')
frame = Frame(M, 17 * mm, W - 2 * M, H - 35 * mm, id='f', leftPadding=0, rightPadding=0, topPadding=0, bottomPadding=0)
doc.addPageTemplates([PageTemplate(id='cover', frames=[frame], onPage=cover), PageTemplate(id='body', frames=[frame], onPage=page)])

S = []
S += [NextPageTemplate('body'), PageBreak()]

# Indice
S += [P('Contents', 'h1'), Spacer(1, 4)]
toc = [
    ('1', 'Getting started', 'opening Tasso, Gaming or Office, the Windows warning'),
    ('2', 'Home', 'the PC score, the CLEAN UP button, the sensors'),
    ('3', 'The 5 cleanups', 'Classic, Normal, Medium, Deep, Extreme'),
    ('4', 'Speed', 'modes, Turbo, startup apps'),
    ('5', 'Programs', 'uninstalling properly, preinstalled apps'),
    ('6', 'Files', 'duplicates, large files, empty folders, shred and recover'),
    ('7', 'Fix problems', 'the 100 office problems, games and Windows'),
    ('8', 'Internet, disk and games', 'ping, Internet speed, disk, games'),
    ('9', 'Hardware', 'components, sensors, BIOS, graphics card'),
    ('10', 'Settings', 'themes, start with Windows, alerts'),
    ('11', 'Turning Tasso off and putting things back', 'the switch and the restore'),
    ('12', 'Safety and backup', 'how to go back if you need to'),
    ('13', 'Frequently asked questions', 'antivirus, USB stick, sending it to a friend'),
]
rows = [[Paragraph('<font name="PopB" color="#0FA46E">%s</font>' % n, st['toc']), Paragraph('<b>%s</b>' % t, st['toc']), Paragraph(d, st['small'])] for n, t, d in toc]
tt = Table(rows, colWidths=[12 * mm, 62 * mm, W - 2 * M - 74 * mm])
tt.setStyle(TableStyle([('LINEBELOW', (0, 0), (-1, -1), 0.5, LINE), ('VALIGN', (0, 0), (-1, -1), 'MIDDLE'), ('TOPPADDING', (0, 0), (-1, -1), 4), ('BOTTOMPADDING', (0, 0), (-1, -1), 4)]))
S += [tt, Spacer(1, 10)]
S += [box('In short', ['Tasso is a free program that keeps your PC clean, fast and tidy. It uses Microsoft\'s official tools, makes an <b>automatic backup</b> before the big cleanups, and never deletes your personal files without asking you first: whatever you choose to delete goes to the <b>Recycle Bin</b>.'], GREEN, GREEN_BG)]

# 1 Primi passi
S += chapter(1, 'Getting started', 'the basics')
S += [P('<b>What you need</b>', 'h2')]
S += bullets(['A PC with 64-bit <b>Windows 10 or Windows 11</b>.',
              '<b>Microsoft Edge WebView2</b>: it\'s already built into Windows 11 and almost every Windows 10 PC. If it\'s missing, Tasso opens the official Microsoft page for you so you can install it for free.',
              '<b>Administrator</b> rights on the PC (the ones you usually have on your home computer).'])
S += [P('<b>How to open it</b>', 'h2')]
S += steps(['Download <b>Tasso.zip</b> from github.com/sergiotassoni/tasso, right-click the file and choose <b>Extract All</b>.',
            'Open the <b>Tasso</b> folder and double-click <b>Tasso.exe</b>.',
            'Windows asks "Do you want to allow this app to make changes to your device?": click <b>Yes</b>. Tasso needs this to clean and repair the system.',
            'If the blue <b>"Windows protected your PC"</b> screen shows up, click <b>More info</b> and then <b>Run anyway</b> (see the box below).'])
S += [box('Why does Windows say "Windows protected your PC"?', ['Tasso is <b>digitally signed</b> (see Properties &gt; Digital Signatures), but Windows shows this warning for programs that haven\'t been downloaded by enough people yet. It doesn\'t mean Tasso is a virus: Windows just doesn\'t know it yet. The more people use it, the less often it shows up.'], AMBER, AMBER_BG)]
S += [P('<b>First-time setup</b>', 'h2'),
      P('The first time, Tasso asks you <b>how you\'ll use the PC</b>. Click the right card (Tasso shows you right away what it will look like) and then <b>Continue</b>:')]
S += [table([['Mode', 'What Tasso looks like and what it sets'],
             ['<b>Gaming</b>', 'All of Tasso: games, FPS, server ping, Turbo, Performance+, dark and colorful themes. It puts the PC in Gaming mode.'],
             ['<b>Office</b>', 'Simpler and calmer: soft, light colors, a cuter badger, no games, FPS or ping. In Fix problems you\'ll find the <b>100 most common office problems</b> right away. It puts the PC in Normal mode and sets up a weekly scheduled cleanup.']],
            [32 * mm, W - 2 * M - 32 * mm])]
S += [P('You can switch modes whenever you want with the <b>Gaming / Office</b> button at the top left, under the logo (or from <b>Settings</b>). The button only changes how Tasso looks; it doesn\'t touch the PC. Each mode remembers its own theme.', 'small')]

# 2 Home
S += chapter(2, 'Home', 'at a glance')
S += bullets(['<b>The PC score</b> (from 0 to 100%): it takes into account junk files, free space, startup apps, the state of your disks and how long it\'s been since the last cleanup.',
              '<b>PC health</b>: the score from 0 to 100 with tips to improve it (see chapter 8).',
              '<b>The round CLEAN UP button</b>: starts the recommended cleanup (Medium). First it shows you what it will do and asks you to confirm.',
              '<b>Choose the power</b>: the five cards to pick the cleanup you want (see chapter 3).',
              '<b>Scheduled cleanup</b>: below the cleanups. Choose the day, the time, the level (Classic, Normal or Medium; Deep or Extreme too if you want, but they take longer) and whether to run it every week or every 2 weeks. If the PC is off at that time, Tasso cleans as soon as you turn it back on. In a scheduled cleanup Tasso is more careful: from the Recycle Bin it only removes what was thrown away more than 7 days ago, and it skips the long jobs if the laptop is running on battery.',
              '<b>The sensors</b> at the bottom: how hard the processor is working, the RAM in use, how full the disk is and, with NVIDIA cards, the graphics card temperature.',
              '<b>The active mode</b> is shown at the top right (Normal, Gaming, FPS, Movie, Quiet).'])

# 3 Pulizie
S += chapter(3, 'The 5 cleanups', 'the heart of Tasso')
S += [table([['Cleanup', 'Time', 'What it does', 'When'],
             ['<b>1 Classic</b>', '1 min', 'Your temporary files, the Recycle Bin and photo thumbnails.', 'Whenever you want, even every day.'],
             ['<b>2 Normal</b>', '3 min', 'Everything in Classic + Windows temporary files, browser cache, DNS cache.', 'Every week.'],
             ['<b>3 Medium</b>', '16 min', 'Everything in Normal + <b>backup</b>, app cache (Discord, Steam, Teams, Epic) and graphics card cache, Windows Update cache, useless system files, driver leftovers, launcher cache. This is the <b>recommended</b> one.', 'Every 2 weeks.'],
             ['<b>4 Deep</b>', '55 min', 'Everything in Medium + drivers and Windows updates, old graphics drivers, startup apps, browser extensions, disk health, security, defragmentation or TRIM, antivirus.', 'Once a month.'],
             ['<b>5 Extreme</b>', '90 min', 'Everything in Deep + Windows repair (DISM and SFC), stability (crashes and hardware errors), a check of every program, disk errors, devices.', 'When the PC has problems, or every 2-3 months.']],
            [28 * mm, 17 * mm, 90 * mm, W - 2 * M - 135 * mm])]
S += [box('The PC score, before and after', ['In <b>Cleanup</b> you\'ll find the <b>PC score</b>: in a few seconds it measures the speed of the processor, memory and disk, and how "light" Windows is (memory in use, background and startup programs, free space, junk files). All scores go <b>from 1 to 100</b> (70 = an average gaming PC of today), and there\'s an <b>overall score</b> with a <b>rank</b>: slow PC (below 40), office PC (40-59), good PC (60-79), fast PC (80-94), top (95 and up). If you turn on "Measure before and after each cleanup", you can see how much it helped. Honestly: cleanup mostly improves lightness; the speed of the parts depends on the hardware.'], GREEN, GREEN_BG)]
S += [P('<b>During the cleanup</b>', 'h2')]
S += bullets(['The bar moves and the messages change: the PC is working, and you can go do something else in the meantime.',
              'On the right you can see the <b>space freed up</b>, the <b>time left</b> and the <b>details</b> of each step.',
              '<b>Stop</b>: Tasso stops as soon as the current step is finished, so nothing is left half done.',
              '<b>Turn off the PC when it\'s done</b>: check the box before you start. At the end the PC turns off after 2 minutes, and you can cancel with the <b>Cancel the shutdown</b> button.',
              'Close your browsers before you start: open ones are skipped.'])
S += [box('When it\'s done', ['You\'ll see the space freed up, how long it took and the full log. If updates were installed or repairs were made, Tasso offers to <b>restart</b>. Every cleanup is saved in a log (Settings, <b>Open the logs folder</b>).'], GREEN, GREEN_BG)]

# 4 Velocità
S += chapter(4, 'Speed', 'snappier')
S += [P('<b>The modes</b>', 'h2')]
S += [table([['Mode', 'What it does'],
             ['<b>Normal</b>', 'A balance between speed and power use, with notifications on. Good for every day.'],
             ['<b>Gaming</b>', 'Power plan at maximum, Windows Game Mode on, notifications paused.'],
             ['<b>Competitive FPS</b>', 'Like Gaming, plus the mouse without acceleration for more precise aim.'],
             ['<b>Movie</b>', 'No notifications, and the screen doesn\'t turn off during the movie.'],
             ['<b>Quiet</b>', 'Processor limited to 70%: less heat and calmer fans.']],
            [36 * mm, W - 2 * M - 36 * mm])]
S += [P('Before turning on a mode, Tasso checks the PC and lets you know if needed: for example if you\'re on a laptop (better to keep the charger plugged in) or if you have little RAM.', 'p')]
S += [P('<b>Turbo</b>', 'h2'),
      P('It closes background programs you don\'t need while you play (OneDrive, Teams, Skype, Dropbox and similar) and frees up memory. It doesn\'t touch your games, Discord or your browser. The closed programs open again by themselves the next time you start the PC.')]
S += [P('<b>Startup apps</b>', 'h2'),
      P('Every program that starts when you turn on the PC has its own <b>switch</b>. Turn off the ones you don\'t need right away: Windows will start faster. The program stays installed; it just doesn\'t start by itself.'),
      P('<b>Startup guard</b>: if a new program quietly adds itself to startup, Tasso lets you know with a small alert.', 'p')]

# 5 Programmi
S += chapter(5, 'Programs', 'install and remove')
S += [P('<b>Uninstalling properly</b>', 'h2')]
S += steps(['Go to <b>Programs</b> and find the program (you can type its name in the search bar).',
            'Click <b>Uninstall</b> and follow the windows of the uninstaller.',
            'At the end, Tasso looks for <b>leftover folders</b> and asks you whether to move them to the Recycle Bin.'])
S += [P('<b>Preinstalled apps</b>', 'h2'),
      P('These are the little games and ad apps that Windows installs by itself (Candy Crush, TikTok, Solitaire, News and similar). The ones already checked are useless for almost everyone; remove the others only if you don\'t use them. If you need one again, you can reinstall it for free from the Microsoft Store.')]

# 6 File
S += chapter(6, 'Files', 'order and space')
S += steps(['Go to <b>Files</b> and click <b>Scan my files</b>. Tasso looks in Desktop, Documents, Pictures, Videos, Music and Downloads.',
            'It shows you <b>duplicate files</b> (the first one in each group stays, the copies are checked), the <b>largest files</b> (you choose: over 100 MB, 300 MB or 1 GB; the ones not touched in over a year are marked), <b>empty folders</b> and <b>broken shortcuts</b>. With <b>Add a folder or drive</b> you can also check another folder or an external drive.',
            'Check the boxes and click <b>Move selected to Recycle Bin</b>. Until you empty the Recycle Bin, you can get everything back.'])
S += [table([['Tool', 'What it\'s for'],
             ['<b>Shred files</b>', 'For private documents, even several files at once: they get overwritten and deleted, and <b>can no longer be recovered</b>. Honestly: on SSDs Windows moves data around by itself, so a 100% guarantee doesn\'t exist; Tasso also asks the disk to clear free space right away (TRIM). For a PC you\'re selling, also use Wipe free space.'],
             ['<b>Recover deleted files</b>', 'Opens the Recycle Bin and Microsoft\'s free tool with its guide. Tip: don\'t save new files until you\'ve recovered the ones you need.'],
             ['<b>Wipe free space</b>', 'Makes it impossible to recover files that were already deleted in the past. Useful before you sell or give away the PC. It can take hours.']],
            [44 * mm, W - 2 * M - 44 * mm])]

# 7 Risolvi
S += chapter(7, 'Fix problems', 'one click on the problem')
S += [table([['Problem', 'What Tasso does'],
             ['<b>I can\'t hear any sound</b>', 'Restarts the Windows audio services and checks the sound cards.'],
             ['<b>The internet isn\'t working</b>', 'Clears the cache, renews the connection and resets the network settings. The internet drops for a few seconds; a restart is needed at the end.'],
             ['<b>Windows Update is stuck</b>', 'Resets the Windows Update components when updates won\'t install.'],
             ['<b>Windows shows errors</b>', 'Checks and repairs the system files (DISM and SFC). 20-40 minutes.'],
             ['<b>A device isn\'t working</b>', 'Checks the mouse, keyboard, network, display and USB, and tells you which one has problems.'],
             ['<b>The disk is acting up</b>', 'Checks the disks for errors, health, wear and temperature.'],
             ['<b>Printer, Store, Start menu</b>', 'Clears a stuck print queue, repairs the Store and apps, and restarts search and the Start menu.']],
            [46 * mm, W - 2 * M - 46 * mm])]
S += [P('<b>Game problems</b>', 'h2'),
      P('The 13 most common problems gamers run into, each with its own button. Tasso always explains what it does and never touches games, saves or anti-cheat files.')]
S += [table([['Problem', 'What Tasso does'],
             ['<b>The game stutters</b>', 'Clears the graphics caches (the DirectX, AMD, NVIDIA and Intel "shaders"), turns on Game Mode and tells you if the PC is saving power.'],
             ['<b>Low FPS</b>', 'FPS means frames per second. It sets the power to maximum, turns off hidden background recording and tells you what\'s stealing power (battery, full memory, heavy programs).'],
             ['<b>High ping or online lag</b>', 'Clears the internet address cache, sets the network back the way Windows wants it, stops the hidden sharing of updates and tells you if your Wi-Fi is slowing you down.'],
             ['<b>The game won\'t start</b>', 'Installs or repairs Visual C++, DirectX and .NET from Microsoft\'s official website: these are the Windows "pieces" that games need.'],
             ['<b>Steam, Epic and launchers</b>', 'Closes stuck launchers and clears their saved pages (white or black screens). Games and accounts stay.'],
             ['<b>Black screen or resolution</b>', 'Restarts the graphics card in 2 seconds and explains the useful keys (Alt+Enter, Win+P).'],
             ['<b>Controller, headset, microphone</b>', 'Restarts stuck devices and Windows audio, and checks whether the microphone is blocked by privacy settings.'],
             ['<b>Xbox and Game Pass</b>', 'Restarts the Xbox services and repairs the Xbox app and the Store.'],
             ['<b>Anti-cheat error</b>', 'Checks Vanguard, Easy Anti-Cheat, BattlEye and FACEIT, and turns them back on if another program turned them off.'],
             ['<b>Discord</b>', 'Closes Discord if it\'s stuck, clears its cache (messages and sign-in stay) and checks the microphone.'],
             ['<b>Mouse aim isn\'t precise</b>', 'Turns off Windows pointer "acceleration": the same movement always moves your aim by the same amount. You can undo it with a double-click.'],
             ['<b>The game minimizes by itself</b>', 'Turns back on the Windows protection that stops other programs from "stealing" the window, and tells you which open programs usually do it.'],
             ['<b>Game downloads are slow</b>', 'Checks for a "metered" network, speed limits, updates and a full disk, and clears the Steam download cache. Games and saves stay.']],
            [46 * mm, W - 2 * M - 46 * mm])]
S += [P('<b>The 100 office problems</b> (new in 2.6)', 'h2'),
      P('Fix problems is split into three sections: <b>Office</b>, <b>Gaming</b> (only in Gaming mode) and <b>Windows</b>, with <b>a single search</b> at the top: type the problem (for example "printer", "zip" or "password") and Tasso shows you only the matching ones, across all sections. The 100 office problems are split into 10 groups that open with one click (printing, Word/Excel/Adobe, Outlook, Teams, files and zip, network, screens, mouse and keyboard, Windows, accounts and security). In Office mode the groups are open and this section comes first; in Gaming mode the groups are closed and this section comes after the games and Windows ones. Tasso never deletes documents (at most it moves them to a folder in Documents), saves the settings before changing them and <b>never closes Word, Excel or Outlook while you\'re working in them</b>: it asks you to save and close them yourself (the only exception is Outlook stuck for more than 2 minutes with no window). The <b>Check</b> buttons (instead of Fix) are for cases that can\'t be fixed automatically: Tasso finds the cause and explains what to do.')]
S += [table([['Problem', 'What Tasso does'],
             ['<b><font color="#2F6BE0">PRINTING &amp; SCANNERS</font></b>', ''],
             ['<b>The printer says "offline"</b>', 'Restarts the print service, unchecks "Use Printer Offline" and checks whether network printers respond.'],
             ['<b>It always prints on the wrong printer</b>', 'Turns off "Let Windows manage my default printer" (which changes it on its own) and lets you pick the right one once and for all.'],
             ['<b>"Print to PDF" is missing</b>', 'Brings back the "Microsoft Print to PDF" printer, the one that saves any document as a PDF.'],
             ['<b>I can\'t find the printer (or other PCs) on the network</b>', 'Turns on the Windows services that look for printers and PCs on the office network. If the network is set to "Public", it explains why they\'re hidden.'],
             ['<b>The USB printer doesn\'t show up</b>', 'Makes Windows look again for printers connected by cable, and restarts stuck printers and the print service.'],
             ['<b>The scanner doesn\'t work</b>', 'Restarts the Windows service for scanners and cameras, unblocks stuck scanners and opens the scanning program for you.'],
             ['<b><font color="#2F6BE0">WORD, EXCEL, OFFICE &amp; ADOBE</font></b>', ''],
             ['<b>Office freezes or shows errors</b>', 'Runs Microsoft Office\'s official "Quick Repair", no internet needed. Documents and settings stay. Office must be closed.'],
             ['<b>Update Office</b>', 'Downloads and installs the official updates for Word, Excel, Outlook and PowerPoint. Many errors go away this way.'],
             ['<b>I lost a document I hadn\'t saved</b>', 'Looks for the automatic copies made by Word, Excel and PowerPoint in the last 14 days and puts them in a folder in Documents for you. It doesn\'t delete anything.'],
             ['<b>Word or Excel closes as soon as you open it</b>', 'Sets aside the damaged Word template (Normal.dotm; Word makes a new one) and tells you which add-ins start with Office.'],
             ['<b>Office keeps asking you to sign in or says "unlicensed"</b>', 'Clears the Office sign-in and saved passwords (the registry gets a backup copy first): then you sign in once with your account and everything is back to normal.'],
             ['<b>The Teams button is missing in Outlook</b>', 'Turns Outlook\'s "Teams Meeting" add-in back on when it disappears and clears the Office add-in cache.'],
             ['<b>Word or Excel is slow, flickers or stays blank</b>', 'Turns off Office hardware graphics acceleration, the most common cause of blank windows, flickering and stuttering while you scroll. You can undo it.'],
             ['<b>Word or Excel won\'t save to OneDrive or SharePoint</b>', 'Sets aside Office\'s document cache (it gets damaged and blocks online saving, "Upload pending"). Office makes a new one. It doesn\'t delete anything.'],
             ['<b>A downloaded file opens in "Protected View"</b>', 'Explains why Word and Excel open files from the internet as read-only, and how to unblock only the ones you trust. It doesn\'t turn the protection off: it\'s there to stop viruses in emails. Checks and explains.'],
             ['<b>The file is "locked by another user"</b>', 'Closes copies of Word, Excel and PowerPoint left open in the background with no window (often the "other user" is you) and explains what to do if a coworker really has the file open.'],
             ['<b>Wrong dates, decimals or CSV files in Excel</b>', 'Shows the date and number formats Windows gives Excel and opens the settings to pick your country, plus tips for CSV files that end up in one column.'],
             ['<b>Adobe Acrobat Reader won\'t open PDFs or freezes</b>', 'Closes Acrobat if it\'s stuck, sets aside its damaged settings (it starts fresh) and runs Adobe\'s official repair. Your PDFs aren\'t touched.'],
             ['<b><font color="#2F6BE0">OUTLOOK &amp; EMAIL</font></b>', ''],
             ['<b>Outlook won\'t open</b>', 'If Outlook has been stuck for more than 2 minutes with no window, it closes it, then opens it again and fixes the folder pane (the most common cause). Email and folders stay.'],
             ['<b>Outlook search doesn\'t find my emails</b>', 'Makes Windows rebuild the search index from scratch. It takes a few hours with the PC on, then search works fully again.'],
             ['<b>Emails don\'t arrive or don\'t send</b>', 'Checks the internet, Microsoft\'s mail servers and disk space, and tells you where the problem is. You change the account settings yourself.'],
             ['<b>Outlook is slow: the mailbox is too big</b>', 'Measures Outlook\'s mail files, tells you if they\'re too big and what to do to make them smaller. It doesn\'t delete any emails.'],
             ['<b>Outlook keeps asking for the password</b>', 'Removes old Outlook and Office passwords saved in Windows: you enter them again once and they stick.'],
             ['<b>Outlook attachments won\'t open</b>', 'Empties the hidden folder where Outlook opens attachments, when it\'s full and gives an error. The files are moved to Documents, not deleted.'],
             ['<b>Outlook reminders are going crazy</b>', 'Rebuilds the reminders list when old or duplicate reminders show up, or none show up at all. Appointments and tasks stay.'],
             ['<b>Outlook doesn\'t update or shows empty folders</b>', 'Sets aside Outlook\'s local file (.ost): Outlook downloads it again from the server with the right emails. Only for Microsoft 365, Exchange, Outlook.com and IMAP accounts, where the emails are online. It doesn\'t delete anything.'],
             ['<b>Outlook has changed: I want the old one back</b>', 'If Windows switched to the "new Outlook" by itself, it sets classic Outlook back as the default (if it\'s installed) and opens it. The emails are the same: only the program changes.'],
             ['<b>Links in emails won\'t open</b>', 'Checks the default browser, the most common problem when Outlook says "organization\'s policies" or does nothing. It opens the right page so you can choose it again (Windows doesn\'t let programs do this). Checks and explains.'],
             ['<b>"No email program" when I send a file by email</b>', 'Sets Outlook back as the Windows email program, the one used by "Send to > Mail recipient" and by other programs\' "Send by email" buttons. You can undo it.'],
             ['<b><font color="#2F6BE0">TEAMS &amp; MEETINGS</font></b>', ''],
             ['<b>Teams is slow, blank or won\'t open</b>', 'Closes Teams and clears its cache, Microsoft\'s official fix. Chats, files and meetings stay (they\'re online). If you\'re in a meeting, leave it first: Teams will close.'],
             ['<b>People can\'t hear me in meetings</b>', 'Unmutes the microphone and turns up the volume if it\'s at zero, checks that Windows isn\'t blocking the microphone for Teams, Zoom and Meet and, only if Windows can\'t see it, restarts the audio.'],
             ['<b>The webcam doesn\'t work</b>', 'Tells you if the webcam is already being used by another program, turns it back on if it was off, restarts the camera service and checks the privacy settings.'],
             ['<b>Bluetooth headphones won\'t connect</b>', 'Restarts Bluetooth and its audio services. It also explains why Bluetooth headphones sound worse during calls.'],
             ['<b>I can\'t hear the others in meetings</b>', 'Unmutes the speakers and turns up the volume if it\'s at zero, restarts the audio only if Windows can\'t see them, and tells you where to choose the right headphones.'],
             ['<b>Teams and Outlook notifications don\'t arrive</b>', 'Turns Windows notifications and Teams and Outlook notifications back on if they were off, and tells you how to see whether "Do not disturb" is on. You can undo it.'],
             ['<b>The webcam is "in use by another program"</b>', 'Tells you which program is using the webcam or microphone right now (Teams, Zoom, the browser...), so you know what to close. Checks and explains.'],
             ['<b>The volume goes down by itself during calls</b>', 'Tells Windows to stop lowering music and other sounds when you\'re on a call in Teams, Zoom or Skype. You can undo it.'],
             ['<b><font color="#2F6BE0">FILES, FOLDERS &amp; ZIP</font></b>', ''],
             ['<b>Files open with the wrong program</b>', 'Tells you which program opens PDFs, Word and Excel documents, photos, zip files and links, and opens the right page so you can change it (Windows doesn\'t let programs do this, for safety). Checks and explains.'],
             ['<b>.zip files won\'t open or give an error</b>', 'Checks what opens zip files, brings back Windows "compressed folders" if an uninstalled program broke them, and tells you what to do with password-protected zips, blocked files and names that are too long.'],
             ['<b>"The file name is too long"</b>', 'Turns on long paths in Windows (over 260 characters), useful with OneDrive, SharePoint and server folders. It also tells you how to shorten them, because some programs are still stuck with the limit.'],
             ['<b>I can\'t delete or rename a file ("it\'s open in another program")</b>', 'Closes the File Explorer preview and restarts File Explorer: both often keep the file in use. Folder windows will close: if you\'re copying files, wait for it to finish.'],
             ['<b>I deleted a file by mistake</b>', 'Tells you where it might be: Recycle Bin, OneDrive recycle bin, Previous versions, File History. It opens the Recycle Bin. If it\'s not there, try Tasso\'s "Recover deleted files" tool. Checks and explains.'],
             ['<b>A folder or file has disappeared (maybe it\'s hidden)</b>', 'Shows hidden files and folders and, if a virus has hidden the folders on a USB stick (this happens a lot), makes them show up again. You can undo it.'],
             ['<b>Desktop icons that don\'t open anything anymore</b>', 'Finds desktop shortcuts that point to programs or files that no longer exist and moves them to a folder in Documents (it doesn\'t delete them).'],
             ['<b>Folders open slowly or with the wrong view</b>', 'Resets the saved folder views (icons, columns, sort order), which get damaged over time and slow down File Explorer. With a backup. Folders and files aren\'t touched.'],
             ['<b>File Explorer opens on "Home" instead of "This PC"</b>', 'Makes File Explorer open straight to This PC, with drives, USB sticks and network drives in view. You can undo it.'],
             ['<b>The C: drive is full</b>', 'Tells you what takes up the most space (Downloads, Desktop, Documents, Recycle Bin, OneDrive, old Windows) and what you can safely free up. It doesn\'t delete anything by itself. Checks and explains.'],
             ['<b>Windows 11: the right-click menu is short</b>', 'Brings back the full right-click menu (like in Windows 10), without having to click "Show more options" every time. You can undo it.'],
             ['<b><font color="#2F6BE0">NETWORK, WI-FI &amp; VPN</font></b>', ''],
             ['<b>The Wi-Fi keeps dropping</b>', 'Stops Windows from turning off the Wi-Fi to save power (the most common cause) and tells you how strong the signal is.'],
             ['<b>The Wi-Fi won\'t connect</b>', 'Turns the Wi-Fi adapter back on if it\'s off, and restarts the Wi-Fi service and the adapter. Saved networks and passwords stay.'],
             ['<b>The VPN won\'t connect</b>', 'Restarts the Windows VPN services and the services of your office VPN program (FortiClient, GlobalProtect, Cisco...), and sets the correct time.'],
             ['<b>With the VPN on, office websites won\'t open</b>', 'Clears the address cache (DNS), registers the PC on the network again and shows you which DNS servers it\'s using.'],
             ['<b>Network drives have a red X</b>', 'Checks the network drives (Z:, S:...) one by one, tells you which servers respond and opens the reachable ones to reconnect them.'],
             ['<b>The network folder keeps asking for a password</b>', 'Shows you the passwords saved for the office servers and opens the place where you can fix them. You type the new password yourself.'],
             ['<b>The internet works but websites won\'t open</b>', 'Checks the "proxy" (a gateway all internet traffic has to pass through): if it\'s left over from a removed program and doesn\'t work, it turns it off (with a backup).'],
             ['<b>The network cable is plugged in but doesn\'t work</b>', 'Checks the wired network adapter: if it\'s off, it turns it back on; if it\'s slow, it tells you to replace the cable; then it restarts it and asks the router for an address again.'],
             ['<b>Updates and OneDrive stuck: "metered connection"</b>', 'If the Wi-Fi network you use is marked as "metered", Windows and OneDrive won\'t download. It removes the mark from your home or office network (not from your phone\'s hotspot). You can put it back.'],
             ['<b>Internet is slow only on this PC</b>', 'Checks the Wi-Fi signal, cable speed, VPN and programs downloading or uploading in the background (OneDrive, updates). It tells you what\'s slowing things down. Checks and explains.'],
             ['<b>Remote Desktop: black screen, slow, or won\'t connect</b>', 'Clears the Remote Desktop image cache (it causes black screens and bits of the wrong window) and checks whether the computers you connect to respond. Saved connections stay.'],
             ['<b>A work website is blocked for no reason</b>', 'Checks the Windows "hosts" file, where viruses or old programs can block or redirect websites like Microsoft, Google or your bank. It sets aside only the suspicious lines, with a backup.'],
             ['<b>The laptop can\'t get online on a different network</b>', 'If the network adapter has an address or DNS set by hand (from another network) that doesn\'t work here, it switches them back to automatic. It first saves the old values to a file, so you can put them back at the office.'],
             ['<b><font color="#2F6BE0">SCREENS &amp; USB</font></b>', ''],
             ['<b>The second screen is black</b>', 'Looks again for connected screens and sets "Extend" (like Win+P), so the second monitor turns on next to the first.'],
             ['<b>The projector or TV shows nothing</b>', 'Sets "Duplicate" (like Win+P): the projector or TV shows the same screen as the laptop.'],
             ['<b>The screen is rotated</b>', 'Turns screens that are sideways or upside down the right way up again (it happens when you press Ctrl+Alt+arrow keys by mistake).'],
             ['<b>Text is blurry or too small</b>', 'Turns on the Windows fix for blurry programs and sharper text (ClearType). Then it opens the text size setting for you.'],
             ['<b>USB stick, USB drive or dock not working</b>', 'Restarts stuck USB ports, looks for devices again and gives a letter (E:, F:...) to USB sticks that don\'t have one. It never formats anything.'],
             ['<b>Mouse, USB sticks or other USB devices disconnect by themselves</b>', 'Turns off USB port power saving, the most common reason mice, keyboards, USB sticks and docks disconnect. It uses very little extra power. You can undo it.'],
             ['<b>The screen turns off or locks too soon</b>', 'If the screen turns off or asks for the password after 1-2 minutes, it changes the wait to 10 minutes when plugged in and 5 on battery. It also tells you if your office decides this.'],
             ['<b>The brightness changes by itself</b>', 'Turns off Windows automatic brightness (the one that dims the screen using the light sensor) and tells you what else changes it. You can turn it back on in Settings.'],
             ['<b>Win + Shift + S (screenshot) doesn\'t work</b>', 'Checks that the Snipping Tool is there and working, lets you take screenshots with the PrtScn key too, and tells you where your screenshots end up. You can undo it.'],
             ['<b><font color="#2F6BE0">MOUSE, KEYBOARD &amp; LAPTOP</font></b>', ''],
             ['<b>The keyboard types the wrong symbols</b>', 'Sets the main keyboard of your Windows language back as the default and turns off Alt+Shift, the shortcut that switches keyboards by mistake (with a backup).'],
             ['<b>Copy and paste doesn\'t work</b>', 'Restarts the Windows clipboard service (and the Remote Desktop one, if you use it).'],
             ['<b>I want to find things I copied earlier (Win + V)</b>', 'Turns on clipboard history: with Win + V you can find the last things you copied, text and images. It stays only on this PC. You can turn it off whenever you want.'],
             ['<b>The mouse is too slow, too fast, or double-click doesn\'t work</b>', 'Sets pointer speed, double-click and the scroll wheel back to how they are on a new Windows (with a backup) and tells you if the mouse buttons are swapped.'],
             ['<b>The laptop\'s touchpad doesn\'t work</b>', 'Turns the touchpad back on if it was disabled in Device Manager, restarts its driver, and tells you where to turn it on in Windows and which Fn key turns it off.'],
             ['<b>The number pad is off when I turn on the PC</b>', 'Makes Windows start with Num Lock on, so your PIN and the numbers on the right of the keyboard work right away. You can undo it.'],
             ['<b>The keyboard beeps, types slowly, or keys stay pressed</b>', 'Turns off Sticky Keys, Filter Keys and Toggle Keys (they turn on by mistake when you press Shift 5 times or hold it down) and their shortcuts. With a backup.'],
             ['<b>The Windows key doesn\'t work</b>', 'Checks whether the Windows key is blocked by a setting and unblocks it (unless your office decided it), with a backup. It also tells you about the "Gaming" lock that many keyboards have.'],
             ['<b>The laptop battery doesn\'t last long</b>', 'Reads how worn the battery is compared to new and its charge cycles, and tells you what drains it and whether it needs replacing. Checks and explains.'],
             ['<b><font color="#2F6BE0">WINDOWS SLOW OR FROZEN</font></b>', ''],
             ['<b>File Explorer freezes or is slow</b>', 'Restarts File Explorer and checks whether a network drive that isn\'t responding is slowing it down. Open folder windows will close: if you\'re copying files, wait for it to finish.'],
             ['<b>Taskbar frozen or icons missing</b>', 'Rebuilds the Windows icons and restarts the taskbar, the Start menu and the notification area. If you\'re copying files, wait for it to finish.'],
             ['<b>The taskbar hides by itself</b>', 'Turns off "Automatically hide the taskbar", so the bar with Start and your open programs is always visible. You can undo it.'],
             ['<b>The desktop icons have disappeared</b>', 'Shows the desktop icons again if they were hidden, brings back the Recycle Bin (and This PC, if it had been removed) and tells you if your desktop ended up inside OneDrive. You can undo it.'],
             ['<b>A program isn\'t responding</b>', 'Finds programs that have been frozen ("Not responding") for more than 20 seconds and closes them. It never closes Word, Excel or Outlook (they\'re often just saving). Unsaved work in the closed program is lost.'],
             ['<b>The PC is slow right now</b>', 'Looks at what the processor, memory and disk are being used for right now, and tells you which program is the heaviest and what to do. It doesn\'t close anything. Checks and explains.'],
             ['<b>Strange text or missing characters</b>', 'Rebuilds the Windows font cache when text shows up as little squares or in the wrong font. A restart is needed.'],
             ['<b>I can\'t see previews of photos and documents</b>', 'Turns previews in File Explorer back on if they were off and rebuilds damaged thumbnails. If you\'re copying files, wait for it to finish.'],
             ['<b>I can\'t see .pdf .docx at the end of file names</b>', 'Always shows file extensions (.pdf, .docx, .exe). This way you know what a file is and can spot scam attachments like invoice.pdf.exe.'],
             ['<b>The PC takes a long time to start</b>', 'Measures how long Windows takes to start, finds the programs that slow it down and tells you what to turn off. Checks and explains.'],
             ['<b>Turning off the PC doesn\'t make problems go away</b>', 'Turns off "Fast startup": this way "Shut down" really turns Windows off and clears everything like a restart. Starting up may take a few seconds longer. You can undo it.'],
             ['<b>The PC won\'t go to sleep or wakes up by itself</b>', 'Tells you which program or device is keeping the PC awake or woke it up. Checks and explains what to do.'],
             ['<b><font color="#2F6BE0">TIME, ONEDRIVE, ACCOUNTS &amp; SECURITY</font></b>', ''],
             ['<b>The time is wrong (and websites show errors)</b>', 'Sets the correct time from the internet. With the wrong time, websites say "Your connection isn\'t private" and VPN and Office won\'t connect.'],
             ['<b>OneDrive isn\'t syncing</b>', 'Runs OneDrive\'s official "reset": it restarts and checks all files again. No files are deleted, either on the PC or online.'],
             ['<b>Free up space with OneDrive</b>', 'OneDrive files you haven\'t changed in 60 days stay online and stop taking up space on your disk. You can still see them, and they download again when you open them (you need internet).'],
             ['<b>OneDrive has duplicate files with the PC\'s name</b>', 'Finds the copies OneDrive makes when the same file is changed in two places (e.g. "Quote-OFFICEPC.docx") and lists them in a file, so you choose which one to keep. It doesn\'t delete anything. Checks and explains.'],
             ['<b>Check for suspicious programs</b>', 'Runs a quick scan with the Windows antivirus and looks for unsigned programs that start by themselves from unusual folders. It tells you what to look at; it doesn\'t delete anything by itself.'],
             ['<b>The "Activate Windows" message shows up</b>', 'Checks whether Windows is activated and with what license, and explains what to do. Tasso doesn\'t and never will use pirate tools to activate it. Checks and explains.'],
             ['<b>I signed in with a "temporary profile" (empty desktop)</b>', 'Checks whether Windows signed you in with a temporary profile and tells you where your real profile is: your files are usually all still there. Checks and explains.'],
             ['<b>My Windows password is about to expire</b>', 'Tells you when the password of your Windows or office account expires and how to change it beforehand (Ctrl + Alt + Del > Change a password). Tasso doesn\'t see or save the password. Checks and explains.'],
             ['<b>The PC asks for the BitLocker recovery key</b>', 'Checks whether your disks are protected with BitLocker and tells you where to find the recovery key (your Microsoft or office account) before you need it. Tasso doesn\'t show or save the key. Checks and explains.']],
            [56 * mm, W - 2 * M - 56 * mm])]
S += [P('<b>Drivers for the whole PC</b>', 'h2'),
      P('A <b>driver</b> is the "instruction booklet" that tells Windows how to use a part of the PC. In <b>Fix problems</b>, Tasso lists all your devices (AMD Radeon, NVIDIA or Intel graphics card, audio, headphones, mouse, keyboard, network, Bluetooth, webcam, monitor...), marks the ones with problems with a <b>Repair</b> button, looks for new drivers on <b>Windows Update</b> (certified by Microsoft) and installs them. For graphics cards and gaming devices it opens the brand\'s <b>official website</b> (NVIDIA App, AMD, Intel, Logitech, Razer, Corsair, SteelSeries, motherboard). The <b>Windows downloads drivers by itself</b> switch stays as you set it even after a restart. Never use "free driver" websites or programs that "update all your drivers".')]

# 8 Internet, disco e giochi (2.1)
S += chapter(8, 'Internet, disk and games', 'new from 2.1 to 2.5')
S += bullets(['<b>PC health</b> (in Home): a score from 0 to 100 with the most important problems and a button to fix them. It checks free space, startup apps, temporary files, Recycle Bin, antivirus, Windows updates, pending restarts, memory and disk health.',
              '<b>Internet, speed test</b>: download, upload and ping in about 30 seconds, with a simple verdict (good for movies, online games, downloads).',
              '<b>Internet, your connection</b>: how you\'re connected (Wi-Fi or cable) and useful tips, for example "connect to the 5 GHz network" when your router has it, or "use a cable" if the signal is weak. It no longer shows data usage.',
              '<b>Internet, saved Wi-Fi</b>: the networks the PC has connected to and their passwords, hidden until you click Show. Handy when a guest asks for the Wi-Fi password.',
              '<b>Disk map</b>: the folders that take up the most space, largest first, with a button to open them. Tasso only looks: it doesn\'t delete anything.',
              '<b>Automatic game mode</b> (Settings): when you open a game from Steam, Epic, Riot, Xbox and similar, Tasso switches to Gaming mode by itself; one minute after you close it, it puts the previous mode back.'])

S += [P('<b>Games (new in 2.2)</b>', 'h2')]
S += bullets(['<b>Optimize before playing</b>: Tasso shows you the open programs using the most memory. Choose which ones to close (programs with a window close as if you clicked the X, so they ask you to save if needed) and click <b>Optimize and play</b>: it also switches to Gaming mode. When you\'re done playing, click <b>End game</b>: it puts the previous mode back and reopens the closed programs. Windows, drivers, antivirus, Discord, game launchers and streaming programs (OBS, Streamlabs, Twitch Studio, Medal) are never touched, and Tasso only closes what you check. If you minimize the game to hop over to Chrome for a moment, Tasso still counts it as open and the overlay hides until you go back to the game.',
              '<b>In-game overlay</b>: small, see-through text at the top right or left with processor and graphics card usage and temperature, RAM and the time. You can\'t click it. It shows up by itself when you play (with automatic game mode on) and you can show or hide it with <b>Ctrl + Shift + F12</b>. It shows in windowed or "borderless fullscreen" games. The processor temperature shows up if Windows provides it or if LibreHardwareMonitor is open.',
              '<b>FPS</b>: Tasso never gets inside games, because with anti-cheat you risk a ban. To see your FPS, use the official tools: Windows <b>Win + G</b> (Performance) or NVIDIA\'s <b>Alt + R</b>.',
              '<b>Autoclicker</b>: clicks by itself every so many milliseconds, seconds or minutes, a set number of times or forever, with the left, right or middle button, single or double click, wherever the mouse is or at a fixed spot. It can make small mouse movements that stay inside the game window. You start and stop it with a key you choose (F6 by default); <b>Esc</b> always stops it.',
              '<b>Your library</b>: all the games installed with Steam, Epic Games and Riot on one page, with search and a <b>Play</b> button.'])
S += [box('Autoclicker and online games', ['In online games with anti-cheat (League of Legends, Valorant, Fortnite, CS2 and others) the autoclicker is against the rules and can get your account banned. Use it in single-player games, "clicker" games, or wherever it\'s allowed.'], AMBER, AMBER_BG)]

S += [P('<b>What\'s new in 2.6</b>', 'h2')]
S += bullets(['<b>Detect problems</b> (at the top of the menu, right under Home together with Fix problems, and as a button in Home): one click and Tasso checks for 58 different problems (54 in Office mode) in about a minute. <b>It doesn\'t change anything</b>: it only shows you the ones it really found, each with its own Fix button, plus "Fix all at once" to solve them all in one go. Some problems only show up when they happen (for example a black second screen): for those, there\'s Fix problems.',
              '<b>Internet</b>: instead of data usage there\'s now <b>Your connection</b>, with useful tips (for example "connect to the 5 GHz network" when your router has it).',
              '<b>Two modes</b>: Gaming and Office, chosen the first time you open Tasso and changeable whenever you want from the button at the top left. In Office mode Tasso is simpler and calmer (soft, light colors, a cuter badger) and hides games, FPS, ping, Turbo and Performance+.',
              '<b>The 100 most common office problems</b> in Fix problems (printing, Word, Excel, Adobe, Outlook, Teams, files and zip, network, screens, mouse and keyboard, accounts), each with a button and a simple explanation (see chapter 7).',
              '<b>Fix problems split into sections</b> (Office, Gaming, Windows) with <b>a single search</b> for all problems, and <b>3 new game problems</b>: mouse aim, game that minimizes by itself, slow downloads.',
              '<b>PC score from 1 to 100</b> for everything, with the overall score and the rank (from "Slow PC" to "top").',
              '<b>Games</b>: your library is at the top of the page. <b>Performance+</b> reorganized: easy, safe things first, then the ones for experts.',
              '<b>6 new themes</b> for the office: Office, Paper, Sage, Lavender, Night and Slate (8 in all, with Ice and Sand). Gaming and Office each remember their own theme.'])
S += [P('<b>What\'s new in 2.5</b>', 'h2')]
S += bullets(['<b>Game profile</b> (in Games): choose once what you need for gaming (Gaming or Competitive FPS, the gaming commands, the overlay, automatic game mode) and turn it on with <b>one switch</b>. <b>It stays on even if you turn off or restart the PC</b>: every time the PC starts, Tasso checks that everything is the way you chose and puts back whatever has changed (for example the power plan set back to normal by an update). When you turn it off, everything goes back to how it was before.',
              '<b>Per-game power</b>: choose a game and <b>High power</b> (Gaming mode) or <b>Maximum power</b> (Competitive FPS, with the mouse without acceleration). When you open that game Tasso sets it by itself, and when you close it, it puts things back as they were.',
              '<b>Game server ping</b> (in Internet): League of Legends, TFT, Valorant, CS2, Dota 2, Fortnite and other games on Amazon servers. You see the milliseconds, the jitter and the lost packets, and Tasso tells you which server is best for you.',
              '<b>Tune with Tasso</b> (under each game): it figures out whether the game is competitive, graphics-heavy or performance-focused, looks at your PC and suggests the right graphics settings. It applies what it safely can by itself, and you can undo it with <b>Cancel</b>.',
              '<b>Gaming commands</b> explained in plain words: for each one, <b>what it does</b>, <b>when it helps</b> and <b>whether there are any risks</b>. They stay on after a restart; when turned off, they go back to exactly how they were before Tasso.',
              '<b>Program updates</b> (in Programs): the list of programs with a new version, to update one by one or all at once, and the programs\' <b>real icons</b>.',
              '<b>Vault</b> (in the menu on the left): your passwords, encrypted (AES-256) and saved only on this PC. Choose a <b>4-digit PIN</b> (handy but weaker), <b>6-digit</b> (recommended), <b>7 to 12 digits</b> (the safest) or <b>no PIN</b>: it opens by itself with your Windows account, but anyone who uses the PC with your account can see the passwords. It creates strong passwords and checks whether they\'ve been stolen (the Have I Been Pwned service only receives a tiny piece of the fingerprint, never the password).',
              'Richer <b>PC status on Discord</b>, every 3/6/12/24 hours and on request with !stato.'])
S += [box('If you forget your vault PIN', ['Nobody can recover it, not even Tasso: that\'s exactly what makes it safe. All you can do is reset the vault and start over.'], AMBER, AMBER_BG)]

# 9 Hardware
S += chapter(9, 'Hardware', 'know your PC')
S += bullets(['<b>Components</b>: processor, graphics card, motherboard and BIOS, RAM, disks, Windows version.',
              '<b>Real-time sensors</b>: processor and RAM usage, disk space; with <b>NVIDIA</b> cards also temperature, usage, fan and video memory; disk temperature when Windows makes it available.',
              '<b>Graphics card</b>: the button opens the NVIDIA or AMD control panel, or the official page to download it.'])
S += [box('The BIOS: should you update it or not?', ['Tasso shows you the BIOS version and finds the official page of your motherboard maker, but <b>it doesn\'t update it by itself</b>. Update it only if it fixes a problem you have or if the maker recommends it: if the PC turns off in the middle of the update, the motherboard can stop working. Always follow the maker\'s instructions, with the PC plugged in.'], AMBER, AMBER_BG)]

# 9 Impostazioni
S += chapter(10, 'Settings', 'made to fit you')
S += [table([['Setting', 'What it does'],
             ['<b>Appearance (8 themes)</b>', 'Tasso, Aurora, Lava, Neon, Carbon, Beast (dark) and Ice, Sand (light). Click a card and the app changes color right away.'],
             ['<b>Tasso on</b>', 'The main switch: see chapter 11.'],
             ['<b>Start with Windows</b>', 'Tasso starts by itself, minimized, when you turn on the PC.'],
             ['<b>Smart alerts</b>', 'Small alerts at the bottom right (disk almost full, new program at startup). Never in Gaming or FPS mode.'],
             ['<b>Automatic game mode</b>', 'When you open a game, Tasso switches to Gaming mode and, when you close it, puts the previous mode back.'],
             ['<b>PC status on Discord</b>', 'Tasso posts the PC report in a private Discord channel of yours: health score, processor, graphics card, motherboard, RAM, disks, temperatures and internet, with green checkmarks for what\'s fine. Choose how often (3, 6, 12 or 24 hours); it also arrives after every cleanup. You need a webhook URL (Edit Channel, Integrations, Webhooks, Copy Webhook URL) to paste here: the step-by-step guide is inside Tasso. Optional: with a small free bot (guide in Tasso) you type <b>!stato</b> in the channel and the report arrives right away. Discord can only read: it can\'t control the PC.'],
             ['<b>Tasso mode</b>', 'Gaming or Office, like the button at the top left.'],
             ['<b>Write to us</b>', 'Send an idea, a problem or a comment, with up to 2 photos and, if you like, your PC details and your email for a reply. It is also in Info. If the internet is down, Tasso prepares the email for you.'],
             ['<b>Help improve Tasso</b>', 'Anonymous numbers (version, Windows, processor and graphics card brand, RAM, space freed, buttons used, errors). Never your name, files or IP address. Tasso asks once; here you turn it on or off whenever you like.'],
             ['<b>Undo</b>', 'The list of everything Tasso has changed (Windows settings, startup apps, gaming commands, tuned games, game profile), with a button to put each thing back as it was, or everything at once (see chapter 11).']],
            [42 * mm, W - 2 * M - 42 * mm])]

# 10 Spegnere
S += chapter(11, 'Turning Tasso off and putting things back', 'you decide')
S += [P('At the bottom left there\'s the <b>"Tasso is on"</b> button. Click it to turn Tasso off: when it\'s off, Tasso <b>doesn\'t start with Windows, doesn\'t run automatic cleanups and doesn\'t send alerts</b>. When you turn it off, you choose what to do with the settings it changed:')]
S += [table([['Choice', 'What happens'],
             ['<b>Turn off and restore everything as it was</b>', 'The Windows settings changed by Tasso go back to exactly how they were before you used it.'],
             ['<b>Turn off and leave everything as it is</b>', 'Tasso turns off, but the settings stay the way they are now.']],
            [56 * mm, W - 2 * M - 56 * mm])]
S += [box('What goes back and what doesn\'t', ['<b>Goes back as it was:</b> power plan and processor limit, screen turn-off and sleep, notifications, mouse, Game Mode, privacy settings, startup apps you turned on or off from Tasso.',
                                            '<b>Can\'t be undone:</b> deleted files, uninstalled programs and installed updates. For those, there\'s the Windows backup in chapter 12.'], VIOLET, VIOLET_BG)]
S += [box('Turning off or restarting the PC doesn\'t put anything back', ['Everything Tasso changes <b>stays that way</b> even if you turn off or restart the PC. It only goes back to how it was when you decide: from the button above or from <b>Settings → Undo</b>, where you can put things back one at a time or all at once.'], GREEN, GREEN_BG)]
S += [P('To turn Tasso back on, just click the button again, or <b>Turn Tasso back on</b> in Home.', 'p')]

# 11 Sicurezza
S += chapter(12, 'Safety and backup', 'nothing to fear')
S += bullets(['<b>Automatic backup</b>: before Medium, Deep and Extreme cleanups, Tasso creates a Windows <b>restore point</b>.',
              '<b>Your files</b> are never deleted without your OK, and they go to the Recycle Bin.',
              'Browser <b>passwords and cookies</b> aren\'t touched: Tasso only clears the cache.',
              '<b>Drivers</b>: Tasso only installs the ones certified by Microsoft (Windows Update); for everything else it opens the maker\'s official website.',
              '<b>Privacy</b>: the "Privacy: stop ads and tracking" tool has its own "Privacy: restore" button.'])
S += [P('<b>How to go back with the Windows backup</b>', 'h2')]
S += steps(['Press the <b>Windows</b> key, type <b>Create a restore point</b> and open it.',
            'Click <b>System Restore</b> and then <b>Next</b>.',
            'Choose the point called <b>"Tasso - before maintenance"</b> and confirm. The PC restarts the way it was at that moment (your documents aren\'t touched).'])
S += [box('What Tasso doesn\'t do, on purpose', ['<b>Registry cleaning</b>, <b>constant "RAM freeing"</b>, miracle <b>internet boosters</b>, <b>drivers from unknown websites</b>, <b>overclocking</b> and <b>automatic BIOS updates</b>: they either don\'t really help or are too risky to do automatically.'], RED, RED_BG)]

# 12 FAQ
S += chapter(13, 'Frequently asked questions', 'quick answers')
faq = [
    ('My antivirus or Windows blocks it: what do I do?', 'It\'s the warning for programs Windows doesn\'t know yet (chapter 1): click <b>More info</b> and <b>Run anyway</b>. If an antivirus puts it in quarantine, you can report it as a false positive to the antivirus maker; for Microsoft Defender, you can do it at microsoft.com/wdsi/filesubmission.'),
    ('Can I keep it on a USB stick?', 'Yes. Copy the whole Tasso folder to the USB stick and start Tasso.exe from there: there\'s no need to install it. The settings are saved on the PC you use it on.'),
    ('How do I send it to a friend?', 'Send them the permanent link <b>github.com/sergiotassoni/tasso/raw/main/Tasso.zip</b>: it always downloads the latest version, no account needed. Or join the Tasso Discord server, where you\'ll find the link in the #tasso channel.'),
    ('Does it slow down the PC while I play?', 'No. Tasso reads the sensors every 2 seconds only while it\'s open, and only does work when you start something. In Gaming and FPS mode it doesn\'t send alerts.'),
    ('If I turn off the PC, do the settings go back to how they were?', 'No. They stay the way Tasso set them, including the Game profile. They only go back to how they were if you ask from <b>Settings → Undo</b>.'),
    ('How do I update Tasso?', 'When a new version comes out, an alert shows up in Home that takes you to <b>Settings → Tasso updates</b>. Click <b>Update now</b>: Tasso downloads the new version from the official page, checks that the file is the right one and reopens by itself. Your settings stay. If something goes wrong, you can always download the zip by hand.'),
    ('Where does it save its stuff?', 'In the <b>%LOCALAPPDATA%\\Tasso</b> folder: settings, logs of every cleanup and the encrypted vault.'),
    ('A cleanup is taking a long time: is that normal?', 'Yes, for Deep and Extreme: Windows updates, repairs and defragmentation can take a while. If a step is stuck for too long, Tasso stops it by itself and moves on.'),
    ('A white window or a message about WebView2 shows up.', 'Microsoft\'s free component is missing: click OK, the official page to install it opens, then open Tasso again.'),
]
for q, a in faq:
    S += [KeepTogether([P('<b>%s</b>' % q, 'p'), P(a, 'p'), Spacer(1, 2)])]
S += [Spacer(1, 6), box('Coming in future versions', ['Digital signature (it needs a paid certificate), video and music player with subtitles, file converters, recorder for the last 30 seconds of gameplay, stats on hours played, ad blocking, webcam and microphone control.'], BLUE, colors.HexColor('#EAF1FD'))]
S += [Spacer(1, 10), P('Tasso 3.0', 'small')]

doc.build(S)
print('ok')
