// Package i18n holds Go-side UI strings (tray, toasts, the events CSV text)
// in Ukrainian and English. Keys and templates are copied verbatim from the
// copy document (uk and en columns); placeholders are {name}.
package i18n

import (
	"runtime"
	"strings"
	"sync/atomic"
)

// UK maps copy.md keys to Ukrainian templates.
var UK = map[string]string{
	// §12.1 tray tooltip
	"tray.tooltip.line1":             "{station} · {model}",
	"tray.tooltip.line2":             "{soc}% · {stateWithPower}",
	"tray.tooltip.line3.off.toEmpty": "Без світла {duration} · вистачить на {eta}",
	"tray.tooltip.line3.off.noEta":   "Без світла {duration}",
	"tray.tooltip.line3.on.charging": "Світло є · до 100% о {time}",
	"tray.tooltip.line3.on.idle":     "Світло є",
	"tray.tooltip.stale":             "{station} · {model}\nДані застарілі, останні о {time}\n{lastSoc}% на той момент",
	"tray.tooltip.noConnection":      "{station} · {model}\nНемає зв'язку зі станцією\nОстанні дані о {time}",
	"tray.tooltip.connecting":        "{station} · {model}\nПідключаємось…",
	"tray.tooltip.noStations":        "Marstek Monitor\nСтанцію ще не додано",

	// §4.4 states
	"state.withPower.discharging": "розряд {watts} Вт",
	"state.withPower.charging":    "заряд {watts} Вт",
	"state.idle":                  "очікування",

	// §9.2 short durations
	"duration.short.lt1m": "< 1 хв",
	"duration.short.m":    "{m} хв",
	"duration.short.hm":   "{h} год {mm} хв",
	"duration.short.h":    "{h} год",
	"duration.short.dh":   "{d} д {h} год",

	// §12.2 tray menu
	"tray.menu.header":                "{station} · {soc}% · {gridShort}",
	"tray.menu.header.gridOn":         "світло є",
	"tray.menu.header.gridOff":        "без світла {h}:{mm}",
	"tray.menu.header.stale":          "дані о {time}",
	"tray.menu.header.noStations":     "Станцію ще не додано",
	"tray.menu.open":                  "Відкрити Marstek Monitor",
	"tray.menu.dnd":                   "Не турбувати",
	"tray.menu.dnd.1h":                "1 год",
	"tray.menu.dnd.3h":                "3 год",
	"tray.menu.dnd.8h":                "8 год",
	"tray.menu.dnd.untilMorning":      "До ранку ({time})",
	"tray.menu.dnd.untilOff":          "Доки не ввімкну",
	"tray.menu.dnd.active":            "Не турбувати до {time} · Вимкнути",
	"tray.menu.dnd.active.indefinite": "Не турбувати увімкнено · Вимкнути",
	"tray.menu.quit":                  "Вийти",
	"tray.firstMinimize.toast.title":  "Marstek Monitor працює в треї",
	"tray.firstMinimize.toast.body":   "І далі стежить за станцією. Щоб вийти, клацніть іконку правою кнопкою → «Вийти».",
	// Copy §13.7 (macOS port, final review I1): T picks .mac on darwin.
	"tray.firstMinimize.toast.title.mac": "Marstek Monitor працює в рядку меню",
	"tray.firstMinimize.toast.body.mac":  "І далі стежить за станцією. Щоб вийти, клацніть іконку → «Вийти».",

	// §12.3 Windows toasts. copy.md gives title and body per key (suffixes
	// .title/.body here) and the emoji as a separate icon column (.icon).
	"notify.attribution":                 "{station} · {model}",
	"notify.attribution.short":           "{station}",
	"notify.gridLost.icon":               "🕯️",
	"notify.gridLost.title":              "Зникло світло",
	"notify.gridLost.body":               "{time} · заряд {soc}% · вистачить ≈ на {eta}",
	"notify.gridLost.noEta.body":         "{time} · заряд {soc}%",
	"notify.gridLost.approx.title":       "Світла немає",
	"notify.gridLost.approx.body":        "Зникло між {from} і {to}, поки додаток не працював. Заряд {soc}% · вистачить ≈ на {eta}",
	"notify.gridBack.icon":               "💡",
	"notify.gridBack.title":              "Світло з'явилось",
	"notify.gridBack.body":               "Без світла було {duration} · заряд {soc}% · до 100% ≈ о {time}",
	"notify.gridBack.full.body":          "Без світла було {duration} · заряд 100%",
	"notify.gridBack.stationWasOut.body": "Без світла було ≈ {duration}. Станція, ймовірно, розрядилась о {diedAt} · зараз {soc}%",
	"notify.low.icon":                    "🪫",
	"notify.low.title":                   "Низький заряд — {soc}%",
	"notify.low.body":                    "При {watts} Вт вистачить ≈ на {eta}",
	"notify.full.icon":                   "🔋",
	"notify.full.title":                  "Станція заряджена",
	"notify.full.body":                   "100% · {capacity} кВт·год · вистачить ≈ на {eta}",
	"notify.full.noEta.body":             "100% · {capacity} кВт·год",
	"notify.likelyOut.icon":              "🪫",
	"notify.likelyOut.title":             "Станція, ймовірно, розрядилась",
	"notify.likelyOut.body":              "{time} · заряд перед цим {soc}%",
	"notify.gridFlapping.icon":           "🕯️",
	"notify.gridFlapping.title":          "Світло нестабільне",
	// copy.md has one ICU plural string; its branches are split here
	// (.other is the fractional form, kept so every plural has all forms).
	"notify.gridFlapping.body.one":   "За 10 хв зникало {count} раз. Наступне сповіщення — коли стабілізується.",
	"notify.gridFlapping.body.few":   "За 10 хв зникало {count} рази. Наступне сповіщення — коли стабілізується.",
	"notify.gridFlapping.body.many":  "За 10 хв зникало {count} разів. Наступне сповіщення — коли стабілізується.",
	"notify.gridFlapping.body.other": "За 10 хв зникало {count} рази. Наступне сповіщення — коли стабілізується.",

	// §12.3 no connection (the catalogue body names the cloud: v1 uses
	// notify.noConnection.body.v1 below) and the test toast.
	"notify.noConnection.icon":           "📡",
	"notify.noConnection.title":          "Немає зв'язку зі станцією",
	"notify.noConnection.restored.icon":  "📡",
	"notify.noConnection.restored.title": "Зв'язок зі станцією відновлено",
	"notify.noConnection.restored.body":  "Не було {duration}. Заряд {soc}%.",
	"notify.test.icon":                   "🔔",
	"notify.test.title":                  "Тестове сповіщення",
	"notify.test.body":                   "Якщо ви це бачите — сповіщення працюють.",

	// Not in the catalogue (plan 2, Task 2): an outage found after a gap the
	// app was running through, and a lower-bound duration (grid_back).
	"notify.gridLost.approx.body.pcSleep": "Зникло між {from} і {to}, поки ПК спав. Заряд {soc}% · вистачить ≈ на {eta}",
	"notify.gridLost.approx.body.noLink":  "Зникло між {from} і {to}, поки не було зв'язку зі станцією. Заряд {soc}% · вистачить ≈ на {eta}",
	"duration.atLeast.short":              "≥ {duration}",

	// Not in the catalogue (plan 2, Task 6): the v1 body without the cloud;
	// .day when the last data is not from today («вчора 23:00», «19.11 23:00»).
	"notify.noConnection.body.v1":     "Станція не відповідає вже {duration}. Останні дані о {time}.",
	"notify.noConnection.body.v1.day": "Станція не відповідає вже {duration}. Останні дані — {time}.",

	// §9.3 dates (the last data of a no-connection toast from another day).
	"date.yesterday": "вчора {time}",

	// The journal entries' sentences (copy §6 and plan 2's journal keys), as
	// the frontend catalogue has them: the events CSV text column (final
	// review M1, app journalText).
	"conn.event.apiSilent":          "Локальний API станції перестав відповідати",
	"conn.event.lanLost.noFallback": "Втрачено зв'язок по локальній мережі",
	"conn.event.lanRestored":        "Зв'язок по локальній мережі відновлено (не було {duration})",
	"conn.event.ipChanged":          "Станція змінила адресу: тепер {ip}",
	"journal.likelyOut":             "Станція, ймовірно, розрядилась ≈ о {time} (заряд перед цим {soc}%)",
	"journal.cutoff":                "Заряд дійшов до нижньої межі {soc}% — станція, ймовірно, вимкнула вихід",
	"journal.lowCharge":             "Низький заряд — {soc}%",
	"journal.batReadsStopped":       "Читання даних батареї зупинено: після запиту станція перестала відповідати",
	"journal.batReadsResumed":       "Читання даних батареї відновлено",
	"journal.chargeBlocked":         "Станція заборонила заряд (захист батареї)",
	"journal.chargeAllowed":         "Станція дозволила заряд знову",
	"journal.dischargeBlocked":      "Станція заборонила розряд (захист батареї)",
	"journal.dischargeAllowed":      "Станція дозволила розряд знову",
	"journal.fwUnverified":          "Нова прошивка v{firmware} — дані батареї вимкнено, поки не перевірено",
	"journal.fwVerified":            "Прошивку v{firmware} перевірено — дані батареї увімкнено",
	// Not in the catalogue (tray-icon plan, owner request: grid pending):
	// the tray says the same as the window's «Світло» row.
	"tray.tooltip.line3.pendingOn":        "Світло з'являється… перевіряємо",
	"tray.tooltip.line3.pendingOff":       "Світло зникає… перевіряємо",
	"tray.tooltip.line3.pendingUnknown":   "Світло: перевіряємо…",
	"tray.menu.header.gridPendingUnknown": "перевіряємо…",
	"tray.menu.header.gridPendingOn":      "світло з'являється… перевіряємо",
	"tray.menu.header.gridPendingOff":     "світло зникає… перевіряємо",

	// Not in the catalogue (owner acceptance A2): grid_lost across a gap with
	// nothing observed before it (the first run ever): no window to name.
	"notify.gridLost.approx.body.noFrom": "Заряд {soc}% · вистачить ≈ на {eta}",

	// Not in the catalogue (plan 3, Task 1): a short link drop (no gap) and the
	// seconds form of a short duration; {from}/{to} are HH:MM:SS.
	"journal.linkDropped": "Станція не відповідала {duration} ({from}–{to})",
	"duration.short.s":    "{s} с",

	// Copy §13.6 (plan 3, Task 4, owner-approved): the update toast, once per
	// new version. A click opens Settings → Про програму.
	"notify.update.title": "Доступна версія {version}",
	"notify.update.body":  "Натисніть, щоб оновити",
}

var (
	langs   = map[string]map[string]string{"uk": UK, "en": EN}
	current atomic.Pointer[string] // nil = "uk"
	goos    = runtime.GOOS         // tests switch it
)

// PlatformKey is key + ".mac" on darwin when that variant exists (in either
// catalogue), else key: the Go twin of the frontend's pk (copy §13.7).
func PlatformKey(key, goos string) string {
	if goos != "darwin" {
		return key
	}
	mac := key + ".mac"
	if _, ok := UK[mac]; ok {
		return mac
	}
	if _, ok := EN[mac]; ok {
		return mac
	}
	return key
}

// SetLang switches the catalogue: "uk" or "en"; anything else is "en".
// Safe to call from any goroutine.
func SetLang(lang string) {
	if _, ok := langs[lang]; !ok {
		lang = "en"
	}
	current.Store(&lang)
}

// Lang is the current language, "uk" until SetLang.
func Lang() string {
	if l := current.Load(); l != nil {
		return *l
	}
	return "uk"
}

// T renders the template for key in the current language, substituting
// {name} with the following name, value pairs. A key missing in the current
// language takes the en, then the uk text; a key in neither returns itself.
// On macOS a key with a .mac variant renders that variant (PlatformKey).
func T(key string, kv ...string) string {
	key = PlatformKey(key, goos)
	tmpl, ok := langs[Lang()][key]
	if !ok {
		tmpl, ok = EN[key]
	}
	if !ok {
		tmpl, ok = UK[key]
	}
	if !ok {
		return key
	}
	pairs := make([]string, 0, len(kv))
	for i := 0; i+1 < len(kv); i += 2 {
		pairs = append(pairs, "{"+kv[i]+"}", kv[i+1])
	}
	return strings.NewReplacer(pairs...).Replace(tmpl)
}

// Plural is the key of the plural form of n in the current language:
// key + ".one" / ".few" / ".many" (uk), key + ".one" / ".other" (en).
func Plural(key string, n int) string {
	if Lang() != "uk" {
		if n == 1 {
			return key + ".one"
		}
		return key + ".other"
	}
	m10, m100 := n%10, n%100
	switch {
	case m10 == 1 && m100 != 11:
		return key + ".one"
	case m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14):
		return key + ".few"
	}
	return key + ".many"
}
