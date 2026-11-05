TASSO 3.0
Cleans, repairs and boosts your PC. In one click.
=================================================

HOW TO OPEN IT
1. Extract the folder from the zip (right-click > Extract All).
   You can also copy it to a USB stick: Tasso runs from there without installing it.
   (Only if you turn on scheduled cleanup or the game profile, Tasso puts a copy of
   itself in Program Files\Tasso, so Windows can start it on its own.)
2. Double-click Tasso.exe and click "Yes" when Windows asks:
   Tasso needs administrator rights to clean and repair.
   If you're not an administrator on this PC, Windows asks for an administrator's password.
3. If the blue "Windows protected your PC" screen shows up:
   click "More info" and then "Run anyway".
   This version doesn't have a digital signature yet, and Windows shows this
   warning for new unsigned programs. It doesn't mean Tasso is a virus.

WHAT YOU NEED
- 64-bit Windows 10 or Windows 11.
- Microsoft Edge WebView2: it's already built into Windows 11 and almost every
  Windows 10 PC. If it's missing, Tasso opens the official Microsoft page for you
  so you can install it.

TURNING TASSO OFF
At the bottom left there's the "Tasso is on" button. Click it to turn Tasso off:
when it's off, it doesn't start with Windows, doesn't run automatic cleanups and doesn't
send alerts. When you turn it off, you choose: "restore everything as it was before
Tasso" (the Windows settings changed by Tasso go back to how they were) or "leave
everything as it is".
Click it again to turn Tasso back on.

GETTING THE LATEST VERSION
Always at the same link: https://github.com/sergiotassoni/tasso/raw/main/Tasso.zip

WHERE IT SAVES ITS STUFF
%LOCALAPPDATA%\Tasso  (settings, logs of every cleanup and the encrypted vault)

The full manual, with step-by-step explanations, is in the file Manual_Tasso_EN.pdf.
The quick list of features is in the file INDEX.txt and inside Tasso,
under "Info & index".

------------------------------------------------------------
WHAT'S NEW IN 3.0
------------------------------------------------------------
- Safer: the Windows programs Tasso uses always start from the system folder.
- If Tasso ever closes because of a serious error, the reason is saved in
  %LOCALAPPDATA%\Tasso\logs\crash.txt: send it on Discord and I'll fix it.
- Write to us (Settings and Info): send an idea, a problem or a comment, with
  up to 2 photos. I read them all.
- If you like, anonymous numbers to help improve Tasso: it asks once, and you
  can change your mind in Settings. Never your name, files or IP address.
- "Open any file" is not in this version: it comes back, better, in one of
  the next versions.
- Tasso.exe Properties now show the right version.
- Small code cleanups.

------------------------------------------------------------
WHAT'S NEW IN 2.6.2
------------------------------------------------------------
A big check-up of the whole program, with lots of fixes:
- Normal mode now puts back YOUR settings (mouse, screen turn-off, power plan)
  instead of the factory ones.
- More careful scheduled cleanup: from the Recycle Bin it only removes what was thrown
  away more than 7 days ago, and the long jobs don't start if the laptop is on battery.
- Windows repair: it now tells you the real result, and if Windows is waiting for a
  restart, it tells you before starting.
- The backup (restore point) lets you know if it couldn't create a new one.
- Old graphics drivers: the last two are always kept, so you can go back.
- Open any file: it now recognizes folders and shortcuts to folders and opens them
  in File Explorer.
- Autoclicker: it stops if one of the best-known games with anti-cheat is open
  (Valorant, LoL, Fortnite, Apex, Call of Duty, BattlEye), so you don't get banned.
- Performance+: more honest and accurate tips for your processor, your graphics card
  (Radeon too) and your RAM; on laptops it doesn't suggest things that can't be done.
- Safer one-click update, more reliable scheduled tasks, and lots of small fixes to
  text and screens (small screens too).

------------------------------------------------------------
WHAT'S NEW IN 2.6.1
------------------------------------------------------------
- Tasso updates are now in Settings, on a tab of their own. When a new version
  comes out, an alert shows up in Home that takes you there.

------------------------------------------------------------
WHAT'S NEW IN 2.6
------------------------------------------------------------
DETECT PROBLEMS
- The menu is now in groups: Problems, Speed, Files & programs, PC & network, Other.
- At the top of the menu, together with Fix problems (and as a button in Home). One click
  and Tasso checks for 58 different problems in about a minute (printers, internet, Word,
  Excel, Outlook, OneDrive, devices, keyboard, Windows and, in Gaming mode, games).
- It doesn't change anything: it only shows you the problems it really found, each
  with Fix.
- Some problems only show up when they happen (e.g. a black second screen): for
  those, use Fix problems, with the search.
- Internet: instead of data usage there's now "Your connection", with useful tips
  (e.g. "connect to the 5 GHz network" if your router has it, or "use a cable").

TWO MODES: GAMING AND OFFICE
- The first time you open Tasso, you choose Gaming or Office. You can switch whenever you
  want with the button at the top left, under the logo: it only changes how Tasso looks,
  not the PC.
- Gaming: all of Tasso, with games, FPS, server ping, Turbo and Performance+.
- Office: simpler and calmer, soft, light colors, a cuter badger; no games, FPS or
  ping. 6 new themes: Office, Paper, Sage, Lavender, Night and Slate (8 in all, with
  Ice and Sand).
- Gaming: 3 new game problems (mouse aim, game that minimizes by itself, slow
  downloads), your library at the top of the Games page, Performance+ reorganized.
- ONE-CLICK UPDATE: in Settings there's "Tasso updates". When a new version comes out,
  an alert shows up in Home that takes you there: click "Update now", and Tasso
  downloads the new version from the official page, checks that the file is the right
  one and reopens by itself. It works from 2.6 onward.
- Scheduled cleanup: besides Classic, Normal and Medium, you can now also choose
  Deep (about 50 min) or Extreme (about an hour and a half). They aren't turned on
  by themselves: you choose them. If a restart is needed at the end, Tasso doesn't do it
  by itself.

THE 100 OFFICE PROBLEMS (in Fix problems, with the search)
- Fix problems is split into sections: Office, Gaming and Windows, with a single search
  at the top: type the problem (e.g. "printer", "zip", "password") and you'll find it.
- Offline or wrong printers, Print to PDF, network and USB printers, scanners.
- Word, Excel and Adobe: official Office repair, updates, unsaved documents, programs
  that close or stay blank, sign-in and license, file "locked by another user",
  wrong dates and CSV files, Acrobat Reader not working.
- Outlook: won't open, search, email that doesn't arrive or doesn't update, mailbox
  too big, passwords, attachments, reminders, classic Outlook, links and "Send to".
- Teams and meetings: cache, microphone, webcam (including "in use by another program"),
  Bluetooth headphones, speakers, notifications, volume that goes down during calls.
- Files and zip: wrong program, zips that won't open, names too long, locked files,
  files deleted by mistake or hidden, full disk, right-click menu.
- Network: Wi-Fi, cable, VPN, network drives, proxy, "metered" network, slow internet,
  Remote Desktop, blocked websites, laptop on a different network.
- Screens, mouse and keyboard: second monitor, projector, screen that's rotated or
  turns off, brightness, screenshots, USB, mouse, touchpad, Num Lock, Sticky Keys,
  Windows key, battery, clipboard history.
- Windows and accounts: File Explorer, taskbar and icons, frozen programs, PC slow right
  now, Fast startup, wrong time, OneDrive and duplicate files, activation, temporary
  profile, expiring password, BitLocker, suspicious programs.
- Safe: it never deletes documents, saves the settings before changing them and never
  closes Word, Excel or Outlook if they're open. Where something can't be done
  automatically, Tasso says so and explains what to do.

------------------------------------------------------------
WHAT'S NEW IN 2.5
------------------------------------------------------------
CLEANUP
- 5 levels: 1 Classic (one minute), 2 Normal, 3 Medium (recommended), 4 Deep,
  5 Extreme. Each one also does everything the levels before it do.
- Scheduled cleanup in Home: choose the day, time, level (Classic, Normal, Medium; Deep or
  Extreme too if you want) and whether to run it every week or every 2 weeks. If the PC was off,
  it runs as soon as you turn it on. If Tasso is already open, it does the cleanup
  itself without opening a second window.
- PC score: measures the speed of the components and how "light" Windows is, also before
  and after each cleanup, so you can see how much it helped. Since 2.6 all scores go
  from 1 to 100, with the overall score and the rank.
- Duplicate files and large files in other folders too (external drives included), with
  a 100 MB / 300 MB / 1 GB limit and a warning on files not touched in over a year.
- Shred files: several files at once, with an honest explanation about SSDs.

GAMES
- Game profile: choose once what you need (power, gaming commands, overlay, automatic
  game mode) and turn it on with one switch. IT STAYS ON even if you turn off or restart
  the PC: every time the PC starts, Tasso checks and puts back whatever Windows or the
  PC maker's program has changed. It only turns off when you turn it off.
- Per-game power: choose a game and "High power" or "Maximum power": Tasso sets it by
  itself when you open that game and removes it when you close it.
- Game server ping (League of Legends, TFT, Valorant, CS2, Dota 2, Fortnite and other
  games on Amazon servers): ms, jitter and lost packets, with the best server.
- Tune with Tasso: recommended graphics settings for each game, based on the type of
  game and your PC.
- Gaming commands explained in plain words: what it does, when it helps, whether there
  are any risks.

FIX PROBLEMS
- 10 gamer problems with one button each: stuttering game, low FPS, high ping,
  game won't start (official Microsoft Visual C++, DirectX, .NET), Steam/Epic/launchers
  that won't open, black screen or wrong resolution, controller/headset/microphone,
  Xbox app and Game Pass, anti-cheat errors (Vanguard, Easy Anti-Cheat, BattlEye),
  Discord not working or people can't hear you.
- Drivers for the whole PC: AMD Radeon, NVIDIA or Intel graphics card, audio, headphones,
  mouse, keyboard, network, Bluetooth, webcam... Tasso lists them, finds the ones with
  problems (Repair button), looks for new drivers on Windows Update (Microsoft-certified)
  and installs them; for everything else it opens the brand's official website (NVIDIA
  App, AMD, Intel, Logitech, Razer, Corsair, SteelSeries, motherboard...). Never "free
  driver" websites.

PROGRAMS AND FILES
- Program updates: see which programs have a new version and update them one by one
  or all at once (winget, free, already built into Windows).
- Real icons for installed programs, startup programs and open programs.
- Open any file: Tasso looks INSIDE the file and figures out what it really is (even if
  the name is wrong or has no extension). If the right program is missing, it suggests
  a free, reliable one (7-Zip, VLC, LibreOffice, Acrobat Reader...): it installs it
  in one click or opens the official download page for you.
- Startup apps match Task Manager, including Store apps.

SAFETY AND CONVENIENCE
- Undo (Settings): the list of everything Tasso has changed, with a button to put each
  thing back as it was, or everything at once. Turning off or restarting the PC does
  NOT put anything back: things only go back when you decide.
- Vault: 4-digit, 6-digit or 7 to 12-digit PIN, or no PIN (it opens by itself with
  your Windows account: handy, but anyone who uses your account can see the passwords).
- Richer PC status on Discord, every 3/6/12/24 hours and on request with !stato.
- "All tools" opens with all sections expanded.

FIXES
- Internet > Internet usage: the "Refresh" button now shows that it's working and
  the time of the last refresh.
- High performance: on PCs that only have the "Balanced" plan (many new laptops)
  nothing used to happen; now Tasso creates the plan or uses "Best performance".
- Scheduled cleanup: if Tasso was already open, it didn't clean; now it does.
- Gaming commands: when you turn them off, they go back to exactly the value they had
  before Tasso.
- Vault: wrong attempts stay counted even if you close Tasso, and it locks by itself
  after a while if you don't use it.
- Security: web pages, games and downloaded files now open as normal programs,
  not with Tasso's administrator rights.
- Open any file: if a "disguised" file (e.g. photo.jpg) is really a program,
  Tasso says so clearly and doesn't open it by itself.
- High performance: it no longer creates a new copy of the power plan for every game;
  fixed the "Power saver" plan in Quiet mode.
- Maximum power for a game now really switches to Competitive FPS even if you were in
  Gaming mode.
- Anti-cheat: Vanguard (Valorant, LoL) is set back to automatic as Riot asks,
  including the part that starts with Windows (vgk).
- Xbox and anti-cheat: Tasso says "fixed" only if it really fixed it.
- Drivers: "Repair" never leaves the keyboard, mouse or disks turned off; optional
  drivers aren't installed unless you check them.
- The game won't start: Visual C++ that's already installed in a newer version now
  shows as fine.
- Scheduled cleanup runs even when the laptop is on battery.
- Duplicate and large files: never inside Windows.old and the recovery folders.
- The window for choosing a folder opens on top of Tasso, not behind it.
- Interface: tidier layout in a narrow window, "Check for updates" writes in the right place,
  switches always up to date, lots of small fixes.

------------------------------------------------------------
WHAT'S NEW IN 2.4
------------------------------------------------------------
- Subfolders: "All tools" is now split into tidy sections you can collapse.
- Deep uninstaller: after you uninstall a program, Tasso finds the leftovers in the
  folders AND in the registry and removes them (folders to the Recycle Bin, registry with
  an automatic backup). More thorough than a normal uninstall.
- New "Performance+" section (safe overclocking): it reads your components and guides
  you step by step (performance profile, EXPO/XMP for RAM, MSI Afterburner for the
  graphics card, guided undervolting for the processor). Tasso doesn't change clock
  speeds or voltages by itself: it opens the official tools and suggests safety margins.
- One-click backup of photos and documents to a USB stick or external drive (it copies,
  it doesn't move).
- "Create a Desktop shortcut".
- Wi-Fi alert: if you're on the 2.4 GHz band, it suggests switching to 5 GHz.
- Custom alerts: you choose what you want to be alerted about (temperatures, disk,
  memory...).
- New "Beast" theme (fierce purple).
