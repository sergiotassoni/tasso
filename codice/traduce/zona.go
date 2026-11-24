package traduce

import "errors"

// Zona dello schermo dove compaiono i dialoghi (pixel veri del desktop).
type Zona struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

var (
	errZonaVuota = errors.New("zona dei dialoghi non scelta")
	errCattura   = errors.New("non riesco a catturare lo schermo")
)

func zonaDaPunti(x0, y0, x1, y1 int) Zona {
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	return Zona{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
}

func (z Zona) Valida() bool { return z.W >= 20 && z.H >= 12 }

// posizioneFinestrella: dove mettere la finestrella della traduzione rispetto alla zona dei
// dialoghi. pos: "sopra" (predefinito), "sotto", "copri" (sopra l'inglese, lo nasconde) o "lato"
// (a destra, o a sinistra se a destra non c'è spazio). Resta sempre dentro lo schermo.
func posizioneFinestrella(pos string, z Zona, larg, alt, vx, vy, vw, vh int32) (int32, int32) {
	zx, zy, zw, zh := int32(z.X), int32(z.Y), int32(z.W), int32(z.H)
	var x, y int32
	switch pos {
	case "copri", "sottotitoli":
		x, y = zx, zy
	case "sotto":
		x, y = zx, zy+zh+10
		if y+alt > vy+vh {
			y = zy - alt - 10
		}
	case "lato":
		x, y = zx+zw+10, zy
		if x+larg > vx+vw {
			x = zx - larg - 10
		}
	default:
		x, y = zx, zy-alt-10
		if y < vy {
			y = zy + zh + 10
		}
	}
	if x+larg > vx+vw {
		x = vx + vw - larg
	}
	if x < vx {
		x = vx
	}
	if y+alt > vy+vh {
		y = vy + vh - alt
	}
	if y < vy {
		y = vy
	}
	return x, y
}
