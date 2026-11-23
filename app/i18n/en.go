package i18n

// EN maps copy.md keys to English templates (en column, en-GB). Same keys as
// UK, except plural forms: English has .one and .other only.
var EN = map[string]string{
	// §12.1 tray tooltip
	"tray.tooltip.line1":             "{station} · {model}",
	"tray.tooltip.line2":             "{soc}% · {stateWithPower}",
	"tray.tooltip.line3.off.toEmpty": "Grid off {duration} · ~{eta} left",
	"tray.tooltip.line3.off.noEta":   "Grid off {duration}",
	"tray.tooltip.line3.on.charging": "Grid on · full at {time}",
	"tray.tooltip.line3.on.idle":     "Grid on",
	"tray.tooltip.stale":             "{station} · {model}\nOut of date, last update at {time}\n{lastSoc}% at that time",
	"tray.tooltip.noConnection":      "{station} · {model}\nNo connection to station\nLast data at {time}",
	"tray.tooltip.connecting":        "{station} · {model}\nConnecting…",
	"tray.tooltip.noStations":        "Marstek Monitor\nNo station added yet",

	// §4.4 states
	"state.withPower.discharging": "discharging {watts} W",
	"state.withPower.charging":    "charging {watts} W",
	"state.idle":                  "idle",

	// §9.2 short durations
	"duration.short.lt1m": "< 1 min",
	"duration.short.m":    "{m} min",
	"duration.short.hm":   "{h} h {mm} min",
	"duration.short.h":    "{h} h",
	"duration.short.dh":   "{d} d {h} h",

	// §12.2 tray menu
	"tray.menu.header":                "{station} · {soc}% · {gridShort}",
	"tray.menu.header.gridOn":         "grid on",
	"tray.menu.header.gridOff":        "grid off {h}:{mm}",
	"tray.menu.header.stale":          "data from {time}",
	"tray.menu.header.noStations":     "No station added yet",
	"tray.menu.open":                  "Open Marstek Monitor",
	"tray.menu.dnd":                   "Do not disturb",
	"tray.menu.dnd.1h":                "1 hour",
	"tray.menu.dnd.3h":                "3 hours",
	"tray.menu.dnd.8h":                "8 hours",
	"tray.menu.dnd.untilMorning":      "Until morning ({time})",
	"tray.menu.dnd.untilOff":          "Until I turn it back on",
	"tray.menu.dnd.active":            "Do not disturb until {time} · Turn off",
	"tray.menu.dnd.active.indefinite": "Do not disturb is on · Turn off",
	"tray.menu.quit":                  "Quit",
	"tray.firstMinimize.toast.title":  "Marstek Monitor is running in the tray",
	"tray.firstMinimize.toast.body":   "It keeps watching the station. To quit, right-click the icon → Quit.",

	// §12.3 Windows toasts
	"notify.attribution":                 "{station} · {model}",
	"notify.attribution.short":           "{station}",
	"notify.gridLost.icon":               "🕯️",
	"notify.gridLost.title":              "Grid lost",
	"notify.gridLost.body":               "{time} · charge {soc}% · lasts ≈ {eta}",
	"notify.gridLost.noEta.body":         "{time} · charge {soc}%",
	"notify.gridLost.approx.title":       "Grid off",
	"notify.gridLost.approx.body":        "Went off between {from} and {to} while the app wasn't running. Charge {soc}% · lasts ≈ {eta}",
	"notify.gridBack.icon":               "💡",
	"notify.gridBack.title":              "Grid back",
	"notify.gridBack.body":               "Off for {duration} · charge {soc}% · full ≈ {time}",
	"notify.gridBack.full.body":          "Off for {duration} · charge 100%",
	"notify.gridBack.stationWasOut.body": "Off for ≈ {duration}. Station probably ran out at {diedAt} · now {soc}%",
	"notify.low.icon":                    "🪫",
	"notify.low.title":                   "Low charge — {soc}%",
	"notify.low.body":                    "At {watts} W, lasts ≈ {eta}",
	"notify.full.icon":                   "🔋",
	"notify.full.title":                  "Station fully charged",
	"notify.full.body":                   "100% · {capacity} kWh · lasts ≈ {eta}",
	"notify.full.noEta.body":             "100% · {capacity} kWh",
	"notify.likelyOut.icon":              "🪫",
	"notify.likelyOut.title":             "Station probably ran out",
	"notify.likelyOut.body":              "{time} · charge before that {soc}%",
	"notify.gridFlapping.icon":           "🕯️",
	"notify.gridFlapping.title":          "Grid is unstable",
	"notify.gridFlapping.body.one":       "Went off {count} time in 10 min. Next notification once it settles.",
	"notify.gridFlapping.body.other":     "Went off {count} times in 10 min. Next notification once it settles.",

	// §12.3 no connection (v1 body below) and the test toast.
	"notify.noConnection.icon":           "📡",
	"notify.noConnection.title":          "No connection to station",
	"notify.noConnection.restored.icon":  "📡",
	"notify.noConnection.restored.title": "Connection restored",
	"notify.noConnection.restored.body":  "Lost for {duration}. Charge {soc}%.",
	"notify.test.icon":                   "🔔",
	"notify.test.title":                  "Test notification",
	"notify.test.body":                   "If you can see this, notifications are working.",

	// Not in the catalogue (plan 2, Task 2).
	"notify.gridLost.approx.body.pcSleep": "Went off between {from} and {to} while the PC was asleep. Charge {soc}% · lasts ≈ {eta}",
	"notify.gridLost.approx.body.noLink":  "Went off between {from} and {to} while there was no connection to the station. Charge {soc}% · lasts ≈ {eta}",
	"duration.atLeast.short":              "≥ {duration}",

	// Not in the catalogue (plan 2, Task 6).
	"notify.noConnection.body.v1":     "The station hasn't responded for {duration}. Last data at {time}.",
	"notify.noConnection.body.v1.day": "The station hasn't responded for {duration}. Last data: {time}.",

	// §9.3 dates.
	"date.yesterday": "yesterday {time}",

	// The journal entries' sentences (copy §6 and plan 2's journal keys), as
	// the frontend catalogue has them: the events CSV text column (final
	// review M1, app journalText).
	"conn.event.apiSilent":          "The station's local API stopped responding",
	"conn.event.lanLost.noFallback": "Lost local network connection",
	"conn.event.lanRestored":        "Local connection restored (lost for {duration})",
	"conn.event.ipChanged":          "Station address changed to {ip}",
	"journal.likelyOut":             "The station probably ran out at ≈ {time} (charge before that {soc}%)",
	"journal.cutoff":                "Charge reached the {soc}% floor — the station probably turned its output off",
	"journal.lowCharge":             "Low charge — {soc}%",
	"journal.batReadsStopped":       "Battery data reads stopped: the station stopped responding after a read",
	"journal.batReadsResumed":       "Battery data reads resumed",
	"journal.chargeBlocked":         "The station blocked charging (battery protection)",
	"journal.chargeAllowed":         "The station allowed charging again",
	"journal.dischargeBlocked":      "The station blocked discharging (battery protection)",
	"journal.dischargeAllowed":      "The station allowed discharging again",
	"journal.fwUnverified":          "New firmware v{firmware} — battery data off until tested",
	"journal.fwVerified":            "Firmware v{firmware} tested — battery data on",

	// Not in the catalogue (tray-icon plan, owner request: grid pending): see uk.go.
	"tray.tooltip.line3.pendingOn":        "Grid coming back… checking",
	"tray.tooltip.line3.pendingOff":       "Grid going off… checking",
	"tray.tooltip.line3.pendingUnknown":   "Grid: checking…",
	"tray.menu.header.gridPendingUnknown": "checking…",
	"tray.menu.header.gridPendingOn":      "grid coming back… checking",
	"tray.menu.header.gridPendingOff":     "grid going off… checking",

	// Not in the catalogue (owner acceptance A2): see uk.go.
	"notify.gridLost.approx.body.noFrom": "Charge {soc}% · lasts ≈ {eta}",

	// Not in the catalogue (plan 3, Task 1): see uk.go.
	"journal.linkDropped": "The station didn't respond for {duration} ({from}–{to})",
	"duration.short.s":    "{s} s",

	// Copy §13.6 (plan 3, Task 4): see uk.go.
	"notify.update.title": "Version {version} is available",
	"notify.update.body":  "Click to update",
}
