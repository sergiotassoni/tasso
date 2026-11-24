package traduce

// Solo nei giochi: la traduzione dello schermo legge e disegna solo quando in primo piano c'è un
// gioco. Desktop, browser (anche YouTube a schermo intero), Esplora risorse, programmi di Windows
// e di lavoro, Discord, Tasso stesso: niente cattura, niente scritte. Eccezioni a richiesta
// dell'utente (zona scelta a mano, tasto "ritraduci"): una volta sola.

import "strings"

// infoFinestra: cosa so della finestra in primo piano.
type infoFinestra struct {
	Exe           string // nome del programma in minuscolo ("game.exe")
	Percorso      string // percorso completo in minuscolo
	Nostra        bool   // Tasso, Tasso Traduce, WebView2 di Tasso
	DiWindows     bool   // dentro C:\Windows o app di sistema dello Store
	SchermoIntero bool   // copre tutto il monitor (gioco a schermo intero o senza bordi)
	Profilo       bool   // gioco già usato con la traduzione (profilo salvato)
	DaNegozio     bool   // installato da Steam, Epic, GOG, Xbox…
	DiTasso       bool   // gioco della libreria di Tasso o in una cartella di giochi
	Tradotto      bool   // la schermata di questa finestra si sta già traducendo
}

// programmi che non sono mai giochi, oltre a nonGiochi/maiProfilo/nonGiochiNegozi: browser, lettori
// video, chat, ufficio, strumenti
var maiGiochi = map[string]bool{
	"chrome.exe": true, "msedge.exe": true, "firefox.exe": true, "brave.exe": true, "opera.exe": true, "opera_gx.exe": true,
	"vivaldi.exe": true, "arc.exe": true, "chromium.exe": true, "thorium.exe": true, "librewolf.exe": true, "waterfox.exe": true,
	"browser.exe": true, "iexplore.exe": true, "zen.exe": true, "floorp.exe": true, "duckduckgo.exe": true,
	"vlc.exe": true, "mpc-hc.exe": true, "mpc-hc64.exe": true, "mpc-be64.exe": true, "potplayermini64.exe": true,
	"potplayer.exe": true, "wmplayer.exe": true, "video.ui.exe": true, "mpv.exe": true, "kodi.exe": true, "plex.exe": true,
	"netflix.exe": true, "disneyplus.exe": true, "primevideo.exe": true, "spotify.exe": true,
	"discord.exe": true, "telegram.exe": true, "whatsapp.exe": true, "slack.exe": true, "teams.exe": true, "ms-teams.exe": true,
	"zoom.exe": true, "skype.exe": true, "signal.exe": true,
	"winword.exe": true, "excel.exe": true, "powerpnt.exe": true, "outlook.exe": true, "onenote.exe": true, "acrobat.exe": true,
	"acrord32.exe": true, "soffice.bin": true, "notepad.exe": true, "notepad++.exe": true, "code.exe": true,
	"explorer.exe": true, "obs64.exe": true, "steam.exe": true, "steamwebhelper.exe": true, "epicgameslauncher.exe": true,
	"taskmgr.exe": true, "photos.exe": true, "mspaint.exe": true, "snippingtool.exe": true, "claude.exe": true, "chatgpt.exe": true,
}

// decidiGioco: la finestra è di un gioco da tradurre?
func decidiGioco(f infoFinestra) bool {
	exe := strings.ToLower(f.Exe)
	if f.Nostra || exe == "" {
		return false
	}
	if maiGiochi[exe] || nonGiochi[exe] || maiProfilo[exe] || nonGiochiNegozi[exe] {
		return false
	}
	if f.Tradotto || f.Profilo || f.DaNegozio || f.DiTasso {
		return true
	}
	if f.DiWindows {
		return false
	}
	// gioco non conosciuto: a schermo intero o senza bordi, di un programma qualsiasi che non è
	// in nessun elenco dei "mai giochi"
	return f.SchermoIntero
}

// programmi che non sono mai giochi: niente profilo (strumenti di Windows, browser, app di lavoro)
var maiProfilo = map[string]bool{
	"claude.exe": true, "snippingtool.exe": true, "screenclippinghost.exe": true, "screensketch.exe": true,
	"mspaint.exe": true, "notepad++.exe": true, "cmd.exe": true, "powershell.exe": true, "pwsh.exe": true,
	"windowsterminal.exe": true, "wt.exe": true, "conhost.exe": true, "mmc.exe": true, "control.exe": true,
	"systemsettings.exe": true, "textinputhost.exe": true, "searchapp.exe": true, "widgets.exe": true,
	"onedrive.exe": true, "outlook.exe": true, "olk.exe": true, "slack.exe": true, "zoom.exe": true,
	"whatsapp.exe": true, "telegram.exe": true, "spotify.exe": true, "iexplore.exe": true, "vivaldi.exe": true,
	"chatgpt.exe": true, "photos.exe": true, "calculatorapp.exe": true, "msteams.exe": true, "regedit.exe": true,
}

// programmi che non sono giochi anche se a schermo intero
var nonGiochi = map[string]bool{
	"chrome.exe": true, "msedge.exe": true, "firefox.exe": true, "brave.exe": true, "opera.exe": true,
	"discord.exe": true, "code.exe": true, "notepad.exe": true, "explorer.exe": true, "teams.exe": true,
	"ms-teams.exe": true, "obs64.exe": true, "steamwebhelper.exe": true, "steam.exe": true,
	"epicgameslauncher.exe": true, "applicationframehost.exe": true, "searchhost.exe": true,
	"startmenuexperiencehost.exe": true, "shellexperiencehost.exe": true, "lockapp.exe": true,
	"taskmgr.exe": true, "winword.exe": true, "excel.exe": true, "powerpnt.exe": true, "acrobat.exe": true,
	"vlc.exe": false, // i video player vanno bene: i sottotitoli si traducono
}
