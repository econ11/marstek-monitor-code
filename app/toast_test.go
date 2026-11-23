package app

import (
	"encoding/xml"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/econ11/marstek-monitor-code/app/i18n"
	"github.com/econ11/marstek-monitor-code/core/detect"
	"github.com/econ11/marstek-monitor-code/core/history"
	"github.com/econ11/marstek-monitor-code/core/link"
	"github.com/econ11/marstek-monitor-code/core/model"
	"github.com/econ11/marstek-monitor-code/core/monitor"
	"github.com/econ11/marstek-monitor-code/core/notify"
	"github.com/econ11/marstek-monitor-code/core/store"
)

func wantText(t *testing.T, toast notify.Toast, wantTitle, wantBody string) {
	t.Helper()
	title, body := ToastText(toast)
	if title != wantTitle {
		t.Errorf("title = %q, want %q", title, wantTitle)
	}
	if body != wantBody {
		t.Errorf("body = %q, want %q", body, wantBody)
	}
}

func TestToastTextGridLost(t *testing.T) {
	wantText(t, notify.Toast{Kind: notify.GridLost,
		Args: map[string]string{"time": "14:05", "soc": "95", "lasts": "10 год"}},
		"🕯️ Зникло світло", "14:05 · заряд 95% · вистачить ≈ на 10 год")
}

func TestToastTextGridLostNoEta(t *testing.T) {
	wantText(t, notify.Toast{Kind: notify.GridLost,
		Args: map[string]string{"time": "14:05", "soc": "95"}},
		"🕯️ Зникло світло", "14:05 · заряд 95%")
}

// gapArgs: a grid_lost found across a gap, as the policy gives it (RFC 3339
// bounds); kv adds the other args.
func gapArgs(from, to time.Time, kv ...string) map[string]string {
	a := map[string]string{"approx": "1", "to": to.Format(time.RFC3339)}
	if !from.IsZero() {
		a["from"] = from.Format(time.RFC3339)
	}
	for i := 0; i+1 < len(kv); i += 2 {
		a[kv[i]] = kv[i+1]
	}
	return a
}

// Owner acceptance A2: the toast names the window the grid went off in, not
// the first observation after the gap as its start.
func TestToastTextGridLostApprox(t *testing.T) {
	to := time.Date(2026, 11, 23, 12, 1, 0, 0, time.Local)
	wantText(t, notify.Toast{Kind: notify.GridLost, At: to,
		Args: gapArgs(to.Add(-3*time.Hour-46*time.Minute), to, "soc", "95", "lasts", "10 год")},
		"🕯️ Світла немає",
		"Зникло між 08:15 і 12:01, поки додаток не працював. Заряд 95% · вистачить ≈ на 10 год")
	// {from} on another local day: «вчора 23:40», then «21.11 23:40».
	wantText(t, notify.Toast{Kind: notify.GridLost, At: to,
		Args: gapArgs(time.Date(2026, 11, 22, 23, 40, 0, 0, time.Local), to, "soc", "95")},
		"🕯️ Світла немає",
		"Зникло між вчора 23:40 і 12:01, поки додаток не працював. Заряд 95%")
	wantText(t, notify.Toast{Kind: notify.GridLost, At: to,
		Args: gapArgs(time.Date(2026, 11, 21, 23, 40, 0, 0, time.Local), to, "soc", "95")},
		"🕯️ Світла немає",
		"Зникло між 21.11 23:40 і 12:01, поки додаток не працював. Заряд 95%")
	// The policy's "to" is the outage's first observation, t.At.
	a := gapArgs(to.Add(-time.Hour), to, "soc", "95")
	delete(a, "to")
	wantText(t, notify.Toast{Kind: notify.GridLost, At: to, Args: a},
		"🕯️ Світла немає", "Зникло між 11:01 і 12:01, поки додаток не працював. Заряд 95%")
	// No observation before the gap (the first run ever): no window to name.
	wantText(t, notify.Toast{Kind: notify.GridLost, At: to,
		Args: gapArgs(time.Time{}, to, "soc", "95", "lasts", "10 год")},
		"🕯️ Світла немає", "Заряд 95% · вистачить ≈ на 10 год")
}

func TestToastTextGridLostApproxEnglish(t *testing.T) {
	t.Cleanup(func() { i18n.SetLang("uk") })
	i18n.SetLang("en")
	to := time.Date(2026, 11, 23, 12, 1, 0, 0, time.Local)
	wantText(t, notify.Toast{Kind: notify.GridLost, At: to,
		Args: gapArgs(to.Add(-3*time.Hour-46*time.Minute), to, "soc", "95", "lasts", "10 h")},
		"🕯️ Grid off",
		"Went off between 08:15 and 12:01 while the app wasn't running. Charge 95% · lasts ≈ 10 h")
	wantText(t, notify.Toast{Kind: notify.GridLost, At: to,
		Args: gapArgs(time.Date(2026, 11, 22, 23, 40, 0, 0, time.Local), to, "soc", "95", "approxReason", "pc_sleep")},
		"🕯️ Grid off",
		"Went off between yesterday 23:40 and 12:01 while the PC was asleep. Charge 95%")
	wantText(t, notify.Toast{Kind: notify.GridLost, At: to,
		Args: gapArgs(time.Date(2026, 11, 21, 23, 40, 0, 0, time.Local), to, "soc", "95", "lasts", "10 h", "approxReason", "no_link")},
		"🕯️ Grid off",
		"Went off between 21 Nov 23:40 and 12:01 while there was no connection to the station. Charge 95% · lasts ≈ 10 h")
	wantText(t, notify.Toast{Kind: notify.GridLost, At: to, Args: gapArgs(time.Time{}, to, "soc", "95")},
		"🕯️ Grid off", "Charge 95%")
}

func TestToastTextLikelyDead(t *testing.T) {
	wantText(t, notify.Toast{Kind: notify.LikelyDead,
		Args: map[string]string{"time": "00:55", "soc": "11"}},
		"🪫 Станція, ймовірно, розрядилась", "00:55 · заряд перед цим 11%")
}

func TestToastTextGridBack(t *testing.T) {
	wantText(t, notify.Toast{Kind: notify.GridBack,
		Args: map[string]string{"duration": "2 год 05 хв", "soc": "63", "fullAt": "18:40"}},
		"💡 Світло з'явилось", "Без світла було 2 год 05 хв · заряд 63% · до 100% ≈ о 18:40")
	// Unknown values are left out, never invented.
	wantText(t, notify.Toast{Kind: notify.GridBack,
		Args: map[string]string{"duration": "2 хв", "soc": "63"}},
		"💡 Світло з'явилось", "Без світла було 2 хв · заряд 63%")
	wantText(t, notify.Toast{Kind: notify.GridBack,
		Args: map[string]string{"duration": "2 хв", "soc": "100"}},
		"💡 Світло з'явилось", "Без світла було 2 хв · заряд 100%")
	wantText(t, notify.Toast{Kind: notify.GridBack,
		Args: map[string]string{"duration": "6 год", "soc": "3", "diedAt": "00:55"}},
		"💡 Світло з'явилось", "Без світла було ≈ 6 год. Станція, ймовірно, розрядилась о 00:55 · зараз 3%")
}

func TestToastTextLowFullFlapping(t *testing.T) {
	wantText(t, notify.Toast{Kind: notify.LowCharge,
		Args: map[string]string{"soc": "18", "watts": "400", "lasts": "1 год 50 хв"}},
		"🪫 Низький заряд — 18%", "При 400 Вт вистачить ≈ на 1 год 50 хв")
	wantText(t, notify.Toast{Kind: notify.LowCharge, Args: map[string]string{"soc": "18"}},
		"🪫 Низький заряд — 18%", "")
	wantText(t, notify.Toast{Kind: notify.Full,
		Args: map[string]string{"capacity": "5,12", "lasts": "10 год"}},
		"🔋 Станція заряджена", "100% · 5,12 кВт·год · вистачить ≈ на 10 год")
	wantText(t, notify.Toast{Kind: notify.Full, Args: map[string]string{"capacity": "5,12"}},
		"🔋 Станція заряджена", "100% · 5,12 кВт·год")
	for count, word := range map[string]string{"1": "раз", "3": "рази", "5": "разів", "11": "разів", "22": "рази"} {
		wantText(t, notify.Toast{Kind: notify.GridFlapping, Args: map[string]string{"count": count}},
			"🕯️ Світло нестабільне",
			"За 10 хв зникало "+count+" "+word+". Наступне сповіщення — коли стабілізується.")
	}
}

func TestToastTextTimeFromAt(t *testing.T) {
	at := time.Date(2026, 11, 21, 14, 5, 0, 0, time.Local)
	wantText(t, notify.Toast{Kind: notify.GridLost, At: at, Args: map[string]string{"soc": "95"}},
		"🕯️ Зникло світло", "14:05 · заряд 95%")
}

func TestEtaText(t *testing.T) {
	for d, want := range map[time.Duration]string{
		112 * time.Minute:             "1 год 50 хв", // < 3 h: to 5 min
		47 * time.Minute:              "45 хв",
		2 * time.Minute:               "< 1 хв",
		10*time.Hour + 20*time.Minute: "10 год", // >= 3 h: to the hour
		26*time.Hour + 40*time.Minute: "1 д 3 год",
		2*time.Hour + 58*time.Minute:  "3 год",
		125 * time.Minute:             "2 год 05 хв",
		23*time.Hour + 40*time.Minute: "1 д 0 год",
	} {
		if got := etaText(d); got != want {
			t.Errorf("etaText(%v) = %q, want %q", d, got, want)
		}
	}
}

func f64(v float64) *float64 { return &v }

func TestEnrichToast(t *testing.T) {
	at := time.Date(2026, 11, 21, 14, 5, 0, 0, time.Local)
	eta := 10*time.Hour + 10*time.Minute
	snap := monitor.Snapshot{
		Link:    link.Live,
		Mode:    detect.Discharging,
		Profile: model.Profile{NominalWh: 5120},
		Last:    &model.Sample{BatteryW: f64(-401.4)},
		F:       monitor.Forecasts{ToEmpty: &eta},
	}

	lost := enrichToast(notify.Toast{Kind: notify.GridLost, At: at, Args: map[string]string{"soc": "95"}}, snap, nil)
	wantText(t, lost, "🕯️ Зникло світло", "14:05 · заряд 95% · вистачить ≈ на 10 год")

	low := enrichToast(notify.Toast{Kind: notify.LowCharge, At: at, Args: map[string]string{"soc": "18"}}, snap, nil)
	wantText(t, low, "🪫 Низький заряд — 18%", "При 401 Вт вистачить ≈ на 10 год")

	// Stale data: no forecast in the toast (copy.md 12.3).
	stale := snap
	stale.Link = link.Stale
	lost = enrichToast(notify.Toast{Kind: notify.GridLost, At: at, Args: map[string]string{"soc": "95"}}, stale, nil)
	wantText(t, lost, "🕯️ Зникло світло", "14:05 · заряд 95%")

	end := at.Add(125 * time.Minute)
	outs := []store.Outage{
		{ID: 6, Start: at.Add(-time.Hour)},
		{ID: 7, Start: at, End: &end},
	}
	back := enrichToast(notify.Toast{Kind: notify.GridBack, Tag: "7", At: end, Args: map[string]string{"soc": "63"}},
		monitor.Snapshot{Link: link.Live}, outs)
	wantText(t, back, "💡 Світло з'явилось", "Без світла було 2 год 05 хв · заряд 63%")

	flap := enrichToast(notify.Toast{Kind: notify.GridFlapping, At: at.Add(5 * time.Minute)}, snap, outs)
	wantText(t, flap, "🕯️ Світло нестабільне",
		"За 10 хв зникало 1 раз. Наступне сповіщення — коли стабілізується.")

	full := enrichToast(notify.Toast{Kind: notify.Full, At: at}, snap, nil)
	wantText(t, full, "🔋 Станція заряджена", "100% · 5,12 кВт·год")

	// The policy keeps Args for a later report: the original must not change.
	orig := map[string]string{"soc": "95"}
	enrichToast(notify.Toast{Kind: notify.GridLost, At: at, Args: orig}, snap, nil)
	if len(orig) != 1 {
		t.Errorf("enrichToast mutated Args: %v", orig)
	}
}

func TestToastXML(t *testing.T) {
	doc, err := toastXML("🕯️ Зникло світло", "14:05 · заряд 95% & більше", `<Дім> "]]>`+" · VenusE 3.0", "", "bGF1bmNo", false)
	if err != nil {
		t.Fatal(err)
	}
	var got toastDoc
	if err := xml.Unmarshal([]byte(doc), &got); err != nil {
		t.Fatalf("document does not parse: %v\n%s", err, doc)
	}
	if got.Launch != "bGF1bmNo" || got.Binding.Template != "ToastGeneric" || got.Audio.Silent != "" || got.Audio.Src == "" {
		t.Errorf("unexpected document: %+v", got)
	}
	want := []toastLine{
		{MaxLines: "1", Value: "🕯️ Зникло світло"},
		{Value: "14:05 · заряд 95% & більше"},
		{Placement: "attribution", Value: `<Дім> "]]>` + " · VenusE 3.0"}, // a hostile station name stays text
	}
	if !reflect.DeepEqual(got.Binding.Texts, want) {
		t.Errorf("texts = %+v, want %+v", got.Binding.Texts, want)
	}

	doc, err = toastXML("Заголовок", "", "", "", "x", true)
	if err != nil {
		t.Fatal(err)
	}
	got = toastDoc{}
	if err := xml.Unmarshal([]byte(doc), &got); err != nil {
		t.Fatal(err)
	}
	if got.Audio.Silent != "true" || got.Audio.Src != "" || len(got.Binding.Texts) != 1 {
		t.Errorf("silent document: %+v", got)
	}
}

func TestAttribution(t *testing.T) {
	for want, st := range map[string]store.Station{
		"Дім · VenusE 3.0": {Name: "Дім", Model: "VenusE 3.0"},
		"VenusE 3.0":       {Model: "VenusE 3.0"},
		"Дім":              {Name: "Дім"},
		// No station yet (only the test toast can be sent then).
		"Marstek Monitor": {},
		"Дуже довга назва станції на дачі у бабусі": {Name: "Дуже довга назва станції на дачі у бабусі", Model: "VenusE 3.0"},
	} {
		if got := attribution(monitor.Snapshot{Station: st}); got != want {
			t.Errorf("attribution(%+v) = %q, want %q", st, got, want)
		}
	}
}

func TestToastTextGridBackAtLeast(t *testing.T) {
	at := time.Date(2026, 11, 23, 9, 0, 30, 0, time.Local)
	start := at.Add(-11 * time.Hour)
	o := store.Outage{ID: 7, Start: start, End: &at}
	snap := monitor.Snapshot{Link: link.Live, Previous: &o,
		PreviousBounds: &history.Bounds{EndApprox: true, NotAfter: &at, AtLeast: true, Known: time.Hour}}
	back := enrichToast(notify.Toast{Kind: notify.GridBack, Tag: "7", At: at, Args: map[string]string{"soc": "63"}},
		snap, []store.Outage{o})
	if _, body := ToastText(back); !strings.Contains(body, "Без світла було ≥ 1 год") {
		t.Fatalf("body = %q, want the observed lower bound «≥ 1 год»", body)
	}
}

func TestToastTextGridLostApproxPCSleep(t *testing.T) {
	to := time.Date(2026, 11, 23, 14, 5, 0, 0, time.Local)
	from := to.Add(-2 * time.Hour)
	wantText(t, notify.Toast{Kind: notify.GridLost, At: to,
		Args: gapArgs(from, to, "soc", "95", "lasts", "10 год", "approxReason", "pc_sleep")},
		"🕯️ Світла немає",
		"Зникло між 12:05 і 14:05, поки ПК спав. Заряд 95% · вистачить ≈ на 10 год")
	wantText(t, notify.Toast{Kind: notify.GridLost, At: to,
		Args: gapArgs(from, to, "soc", "95", "approxReason", "no_link")},
		"🕯️ Світла немає",
		"Зникло між 12:05 і 14:05, поки не було зв'язку зі станцією. Заряд 95%")
	// A station that probably ran out was not reachable either.
	wantText(t, notify.Toast{Kind: notify.GridLost, At: to,
		Args: gapArgs(from, to, "soc", "95", "approxReason", "likely_dead")},
		"🕯️ Світла немає",
		"Зникло між 12:05 і 14:05, поки не було зв'язку зі станцією. Заряд 95%")
}

// An exact outage: Known without «≥».
func TestToastTextGridBackExactKnown(t *testing.T) {
	at := time.Date(2026, 11, 23, 9, 0, 0, 0, time.Local)
	start := at.Add(-125 * time.Minute)
	o := store.Outage{ID: 7, Start: start, End: &at}
	snap := monitor.Snapshot{Link: link.Live, Previous: &o,
		PreviousBounds: &history.Bounds{Known: 125 * time.Minute}}
	back := enrichToast(notify.Toast{Kind: notify.GridBack, Tag: "7", At: at, Args: map[string]string{"soc": "63"}},
		snap, []store.Outage{o})
	wantText(t, back, "💡 Світло з'явилось", "Без світла було 2 год 05 хв · заряд 63%")
}

func TestToastTextEnglish(t *testing.T) {
	t.Cleanup(func() { i18n.SetLang("uk") })
	i18n.SetLang("en")
	toast := notify.Toast{Kind: notify.GridLost,
		Args: map[string]string{"time": "14:05", "soc": "95", "lasts": etaText(10 * time.Hour)}}
	if _, title, body := toastParts(toast); title != "Grid lost" || body != "14:05 · charge 95% · lasts ≈ 10 h" {
		t.Errorf("title %q, body %q", title, body)
	}
	wantText(t, toast, "🕯️ Grid lost", "14:05 · charge 95% · lasts ≈ 10 h")
	wantText(t, notify.Toast{Kind: notify.GridFlapping, Args: map[string]string{"count": "1"}},
		"🕯️ Grid is unstable", "Went off 1 time in 10 min. Next notification once it settles.")
	wantText(t, notify.Toast{Kind: notify.GridFlapping, Args: map[string]string{"count": "3"}},
		"🕯️ Grid is unstable", "Went off 3 times in 10 min. Next notification once it settles.")
}

// Numbers in toasts follow the language: decimal point and comma grouping
// in en, decimal comma and U+00A0 grouping in uk.
func TestEnrichToastNumbersPerLanguage(t *testing.T) {
	t.Cleanup(func() { i18n.SetLang("uk") })
	at := time.Date(2026, 11, 21, 14, 5, 0, 0, time.Local)
	eta := 50 * time.Minute
	snap := monitor.Snapshot{
		Link: link.Live, Mode: detect.Discharging,
		Profile: model.Profile{NominalWh: 5120},
		Last:    &model.Sample{BatteryW: f64(-2500)},
		F:       monitor.Forecasts{ToEmpty: &eta},
	}
	full := notify.Toast{Kind: notify.Full, At: at}
	low := notify.Toast{Kind: notify.LowCharge, At: at, Args: map[string]string{"soc": "18"}}

	i18n.SetLang("en")
	wantText(t, enrichToast(full, snap, nil), "🔋 Station fully charged", "100% · 5.12 kWh")
	wantText(t, enrichToast(low, snap, nil), "🪫 Low charge — 18%", "At 2,500 W, lasts ≈ 50 min")

	i18n.SetLang("uk")
	wantText(t, enrichToast(full, snap, nil), "🔋 Станція заряджена", "100% · 5,12 кВт·год")
	wantText(t, enrichToast(low, snap, nil), "🪫 Низький заряд — 18%", "При 2 500 Вт вистачить ≈ на 50 хв")
}

func TestToastTextNoConnectionHasNoCloud(t *testing.T) {
	t.Cleanup(func() { i18n.SetLang("uk") })
	lost := notify.Toast{Kind: notify.NoLink, Args: map[string]string{"duration": "16 хв", "time": "14:05"}}
	restored := notify.Toast{Kind: notify.NoLinkRestored, Args: map[string]string{"duration": "35 хв", "soc": "55"}}
	i18n.SetLang("uk")
	wantText(t, lost, "📡 Немає зв'язку зі станцією", "Станція не відповідає вже 16 хв. Останні дані о 14:05.")
	wantText(t, restored, "📡 Зв'язок зі станцією відновлено", "Не було 35 хв. Заряд 55%.")
	// An unknown value drops its sentence.
	wantText(t, notify.Toast{Kind: notify.NoLinkRestored, Args: map[string]string{"duration": "35 хв"}},
		"📡 Зв'язок зі станцією відновлено", "Не було 35 хв.")
	i18n.SetLang("en")
	wantText(t, notify.Toast{Kind: notify.NoLink, Args: map[string]string{"duration": "16 min", "time": "14:05"}},
		"📡 No connection to station", "The station hasn't responded for 16 min. Last data at 14:05.")
	wantText(t, notify.Toast{Kind: notify.NoLinkRestored, Args: map[string]string{"duration": "35 min", "soc": "55"}},
		"📡 Connection restored", "Lost for 35 min. Charge 55%.")
	for _, lang := range []string{"uk", "en"} {
		i18n.SetLang(lang)
		for _, tt := range []notify.Toast{lost, restored} {
			title, body := ToastText(tt)
			for _, bad := range []string{"хмар", "cloud", "Cloud"} {
				if strings.Contains(title+body, bad) {
					t.Errorf("%s %s: %q / %q contains %q", lang, tt.Kind, title, body, bad)
				}
			}
		}
	}
}

func TestToastTextTest(t *testing.T) {
	t.Cleanup(func() { i18n.SetLang("uk") })
	i18n.SetLang("uk")
	wantText(t, notify.Toast{Kind: notify.Test}, "🔔 Тестове сповіщення", "Якщо ви це бачите — сповіщення працюють.")
	i18n.SetLang("en")
	wantText(t, notify.Toast{Kind: notify.Test}, "🔔 Test notification", "If you can see this, notifications are working.")
}

// The policy hands durations over as Go durations; the toast shows the short
// form in the current language.
func TestEnrichToastNoLinkDuration(t *testing.T) {
	t.Cleanup(func() { i18n.SetLang("uk") })
	i18n.SetLang("uk")
	lost := notify.Toast{Kind: notify.NoLink, Args: map[string]string{"duration": (75 * time.Minute).String(), "time": "14:05"}}
	wantText(t, enrichToast(lost, monitor.Snapshot{}, nil),
		"📡 Немає зв'язку зі станцією", "Станція не відповідає вже 1 год 15 хв. Останні дані о 14:05.")
	if lost.Args["duration"] != "1h15m0s" {
		t.Fatalf("enrichToast modified the caller's Args: %+v", lost.Args)
	}
	i18n.SetLang("en")
	back := notify.Toast{Kind: notify.NoLinkRestored, Args: map[string]string{"duration": (35 * time.Minute).String(), "soc": "55"}}
	wantText(t, enrichToast(back, monitor.Snapshot{}, nil), "📡 Connection restored", "Lost for 35 min. Charge 55%.")
}

func TestToastIDKeepsLinkTag(t *testing.T) {
	if got := toastID(notify.Toast{Kind: notify.NoLink, Tag: "link-1763700000"}); got != "link-1763700000" {
		t.Errorf("link tag: id %q", got)
	}
	if got := toastID(notify.Toast{Kind: notify.GridBack, Tag: "42"}); got != "outage-42" {
		t.Errorf("outage tag: id %q", got)
	}
}

// Review fix: last data from another day names the day.
func TestEnrichToastNoLinkLastDataDay(t *testing.T) {
	t.Cleanup(func() { i18n.SetLang("uk") })
	now := time.Date(2026, 11, 22, 9, 0, 0, 0, time.Local)
	toast := func(last time.Time) notify.Toast {
		return notify.Toast{Kind: notify.NoLink, At: now, Args: map[string]string{
			"duration": now.Sub(last).String(), "time": last.Format("15:04"), "lastAt": last.Format(time.RFC3339)}}
	}
	today := time.Date(2026, 11, 22, 8, 30, 0, 0, time.Local)
	yesterday := time.Date(2026, 11, 21, 23, 0, 0, 0, time.Local)
	older := time.Date(2026, 11, 19, 23, 0, 0, 0, time.Local)
	lastYear := time.Date(2025, 12, 31, 23, 0, 0, 0, time.Local)
	i18n.SetLang("uk")
	wantText(t, enrichToast(toast(today), monitor.Snapshot{}, nil),
		"📡 Немає зв'язку зі станцією", "Станція не відповідає вже 30 хв. Останні дані о 08:30.")
	wantText(t, enrichToast(toast(yesterday), monitor.Snapshot{}, nil),
		"📡 Немає зв'язку зі станцією", "Станція не відповідає вже 10 год. Останні дані — вчора 23:00.")
	wantText(t, enrichToast(toast(older), monitor.Snapshot{}, nil),
		"📡 Немає зв'язку зі станцією", "Станція не відповідає вже 2 д 10 год. Останні дані — 19.11 23:00.")
	_, body := ToastText(enrichToast(toast(lastYear), monitor.Snapshot{}, nil))
	if !strings.HasSuffix(body, "Останні дані — 31.12.2025 23:00.") {
		t.Errorf("last year: %q", body)
	}
	i18n.SetLang("en")
	wantText(t, enrichToast(toast(yesterday), monitor.Snapshot{}, nil),
		"📡 No connection to station", "The station hasn't responded for 10 h. Last data: yesterday 23:00.")
	wantText(t, enrichToast(toast(older), monitor.Snapshot{}, nil),
		"📡 No connection to station", "The station hasn't responded for 2 d 10 h. Last data: 19 Nov 23:00.")
}

// A toast click opens the tab the toast is about (flows §2.8), by Kind and
// the numeric outage id in Tag (never by the toast id's prefix).
func TestToastClickRoutesByKind(t *testing.T) {
	for _, c := range []struct {
		name string
		kind notify.Kind
		tag  string
		open int64 // the outage still going on, 0 = none
		want Navigate
	}{
		{"closed outage, grid back", notify.GridBack, "42", 0, Navigate{View: "outages", OutageID: 42}},
		{"grid lost, outage still on", notify.GridLost, "42", 42, Navigate{View: "overview"}},
		{"grid lost, outage closed since", notify.GridLost, "42", 0, Navigate{View: "outages", OutageID: 42}},
		{"grid lost, another outage on now", notify.GridLost, "41", 42, Navigate{View: "outages", OutageID: 41}},
		{"grid flapping", notify.GridFlapping, "", 42, Navigate{View: "outages"}},
		{"grid back, tag not an outage id", notify.GridBack, "link-1795000000", 0, Navigate{View: "outages"}},
		{"grid lost, no outage id", notify.GridLost, "", 42, Navigate{View: "outages"}},
		{"no connection", notify.NoLink, "link-1795000000", 0, Navigate{View: "overview"}},
		{"connection restored", notify.NoLinkRestored, "link-1795000000", 0, Navigate{View: "overview"}},
		{"test", notify.Test, "", 0, Navigate{View: "settings", Section: "notifications"}},
		{"low charge", notify.LowCharge, "", 0, Navigate{View: "overview"}},
		{"plain toast", "", "", 0, Navigate{View: "overview"}},
	} {
		if got := ToastRoute(c.kind, c.tag, c.open); got != c.want {
			t.Errorf("%s: ToastRoute(%q, %q, %d) = %+v, want %+v", c.name, c.kind, c.tag, c.open, got, c.want)
		}
	}
}

// Task 20c: a grid toast without an outage (the store failed to open one:
// id 0) carries no outage tag: it is placed and identified like an untagged
// toast, never as "outage-0", and its click leads to Outages, not outage 0.
func TestToastWithoutOutageHasNoTag(t *testing.T) {
	at := time.Unix(1795000000, 0)
	for _, k := range []notify.Kind{notify.GridLost, notify.GridBack} {
		tt := notify.Toast{Kind: k, Tag: "0", At: at, StationID: "st"}
		if got := toastTag(tt); got != "" {
			t.Errorf("%s: toastTag = %q, want \"\"", k, got)
		}
		if got, want := toastID(tt), string(k)+"-1795000000"; got != want {
			t.Errorf("%s: toastID = %q, want %q", k, got, want)
		}
		if got := toastPlace(tt).Tag; got != string(k) {
			t.Errorf("%s: toastPlace Tag = %q, want %q", k, got, string(k))
		}
		if got := ToastRoute(k, toastTag(tt), 0); got != (Navigate{View: "outages"}) {
			t.Errorf("%s: route = %+v, want Outages", k, got)
		}
	}
	if got := toastTag(notify.Toast{Kind: notify.GridBack, Tag: "42"}); got != "42" {
		t.Errorf("outage 42: toastTag = %q", got)
	}
	if got := toastTag(notify.Toast{Kind: notify.NoLink, Tag: "link-1795000000"}); got != "link-1795000000" {
		t.Errorf("link tag: toastTag = %q", got)
	}
}
