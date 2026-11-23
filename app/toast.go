package app

import (
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/econ11/marstek-monitor-code/app/i18n"
	"github.com/econ11/marstek-monitor-code/app/trayicon"
	"github.com/econ11/marstek-monitor-code/core/detect"
	"github.com/econ11/marstek-monitor-code/core/link"
	"github.com/econ11/marstek-monitor-code/core/monitor"
	"github.com/econ11/marstek-monitor-code/core/notify"
	"github.com/econ11/marstek-monitor-code/core/store"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
	"github.com/wailsapp/wails/v3/pkg/w32"
	"golang.org/x/sys/windows/registry"
)

// flapWindow mirrors the notify policy: grid_flapping reports the last 10 min.
const flapWindow = 10 * time.Minute

// appName is the toast attribution when there is no station yet.
const appName = "Marstek Monitor"

// ToastText renders a toast (copy.md 12.3) in the current language. The emoji prefixes the title:
// a toast has no separate field for it.
//
// Args used: "time" (default: t.At, local), "soc", "lasts" (forecast),
// "approx" ("1" = the start was not observed: the outage was found after
// the app was not running or after a link gap), "approxReason" (the gap
// reason: app_not_running, pc_sleep, no_link, likely_dead), "from" / "to"
// (approx: RFC 3339, the grid went off between them; "to" defaults to t.At,
// "from" on another day is named with it, «вчора 23:40»), "duration",
// "fullAt", "diedAt", "watts", "capacity", "count". A missing value drops its
// " · " part of the body; nothing is ever invented.
func ToastText(t notify.Toast) (title, body string) {
	icon, title, body := toastParts(t)
	if icon != "" {
		title = icon + " " + title
	}
	return title, body
}

func toastParts(t notify.Toast) (icon, title, body string) {
	a := t.Args
	at := a["time"]
	if at == "" && !t.At.IsZero() {
		at = hhmm(t.At)
	}
	switch t.Kind {
	case notify.GridLost:
		icon, title = i18n.T("notify.gridLost.icon"), i18n.T("notify.gridLost.title")
		kv := []string{"time", at, "soc", a["soc"], "eta", a["lasts"]}
		switch {
		case a["approx"] == "1":
			// Not «since ≈ {time}»: t.At is only the first observation after
			// the gap. The grid went off between the gap's start and it.
			title = i18n.T("notify.gridLost.approx.title")
			to := t.At
			if v, err := time.Parse(time.RFC3339, a["to"]); err == nil {
				to = v
			}
			from, err := time.Parse(time.RFC3339, a["from"])
			key := "notify.gridLost.approx.body"
			switch {
			case err != nil || to.IsZero():
				key += ".noFrom" // nothing observed before the gap
			case store.GapReason(a["approxReason"]) == store.GapPCSleep:
				key += ".pcSleep"
			case store.GapReason(a["approxReason"]) == store.GapNoLink, store.GapReason(a["approxReason"]) == store.GapLikelyDead:
				key += ".noLink"
			}
			if err == nil && !to.IsZero() {
				fromText := dayTime(from, to) // «вчора 23:40» on another local day
				if fromText == "" {
					fromText = hhmm(from)
				}
				kv = append(kv, "from", fromText, "to", hhmm(to))
			}
			body = segments(key, kv...)
		case a["lasts"] == "":
			body = segments("notify.gridLost.noEta.body", kv...)
		default:
			body = segments("notify.gridLost.body", kv...)
		}
	case notify.GridBack:
		icon, title = i18n.T("notify.gridBack.icon"), i18n.T("notify.gridBack.title")
		kv := []string{"duration", a["duration"], "soc", a["soc"], "time", a["fullAt"], "diedAt", a["diedAt"]}
		switch {
		case a["diedAt"] != "":
			body = segments("notify.gridBack.stationWasOut.body", kv...)
		case a["soc"] == "100":
			body = segments("notify.gridBack.full.body", kv...)
		default:
			body = segments("notify.gridBack.body", kv...)
		}
	case notify.GridFlapping:
		icon, title = i18n.T("notify.gridFlapping.icon"), i18n.T("notify.gridFlapping.title")
		if n, err := strconv.Atoi(a["count"]); err == nil {
			body = i18n.T(i18n.Plural("notify.gridFlapping.body", n), "count", a["count"])
		}
	case notify.LowCharge:
		icon, title = i18n.T("notify.low.icon"), i18n.T("notify.low.title", "soc", a["soc"])
		body = segments("notify.low.body", "watts", a["watts"], "eta", a["lasts"])
	case notify.Full:
		icon, title = i18n.T("notify.full.icon"), i18n.T("notify.full.title")
		key := "notify.full.body"
		if a["lasts"] == "" {
			key = "notify.full.noEta.body"
		}
		body = segments(key, "capacity", a["capacity"], "eta", a["lasts"])
	case notify.LikelyDead:
		icon, title = i18n.T("notify.likelyOut.icon"), i18n.T("notify.likelyOut.title")
		body = segments("notify.likelyOut.body", "time", at, "soc", a["soc"])
	case notify.NoLink:
		icon, title = i18n.T("notify.noConnection.icon"), i18n.T("notify.noConnection.title")
		// Only the time of the last data, never the toast's own time.
		key := "notify.noConnection.body.v1"
		if a["timeDay"] == "1" { // "time" names another day: «вчора 23:00»
			key += ".day"
		}
		body = sentences(key, "duration", a["duration"], "time", a["time"])
	case notify.NoLinkRestored:
		icon, title = i18n.T("notify.noConnection.restored.icon"), i18n.T("notify.noConnection.restored.title")
		body = sentences("notify.noConnection.restored.body", "duration", a["duration"], "soc", a["soc"])
	case notify.Test:
		icon, title, body = i18n.T("notify.test.icon"), i18n.T("notify.test.title"), i18n.T("notify.test.body")
	case notify.Update:
		title, body = i18n.T("notify.update.title", "version", a["version"]), i18n.T("notify.update.body")
	default:
		title = string(t.Kind)
	}
	return icon, title, body
}

// segments renders the template for key part by part (parts are joined with
// " · ") and leaves out every part with an empty value.
func segments(key string, kv ...string) string { return parts(key, " · ", kv...) }

// sentences is segments for templates of full sentences ending in ".":
// a sentence with an empty value is left out.
func sentences(key string, kv ...string) string {
	s := parts(key, ". ", kv...)
	if s != "" && !strings.HasSuffix(s, ".") {
		s += "." // the last sentence, with its full stop, was left out
	}
	return s
}

func parts(key, sep string, kv ...string) string {
	var out []string
part:
	for _, p := range strings.Split(i18n.T(key), sep) {
		for i := 0; i+1 < len(kv); i += 2 {
			if kv[i+1] == "" && strings.Contains(p, "{"+kv[i]+"}") {
				continue part
			}
		}
		out = append(out, p)
	}
	pairs := make([]string, 0, len(kv))
	for i := 0; i+1 < len(kv); i += 2 {
		pairs = append(pairs, "{"+kv[i]+"}", kv[i+1])
	}
	return strings.NewReplacer(pairs...).Replace(strings.Join(out, sep))
}

func hhmm(t time.Time) string { return t.Local().Format("15:04") }

// dayTime names t relative to now's local day (copy.md §9.3): "" for today
// (the plain time is enough), date.yesterday («вчора 23:00»), else
// date.shortTime («19.11 23:00» / «19 Nov 23:00») or, in another year,
// date.shortYear with the time.
func dayTime(t, now time.Time) string {
	t, now = t.Local(), now.Local()
	y, m, d := t.Date()
	day := func(u time.Time) bool { uy, um, ud := u.Date(); return uy == y && um == m && ud == d }
	switch {
	case day(now):
		return ""
	case day(now.AddDate(0, 0, -1)):
		return i18n.T("date.yesterday", "time", hhmm(t))
	}
	layout, year := "02.01", ".2006" // uk
	if i18n.Lang() != "uk" {
		layout, year = "2 Jan", " 2006" // en-GB
	}
	if y != now.Year() {
		layout += year
	}
	return t.Format(layout) + " " + hhmm(t)
}

// etaText is a forecast rounded as copy.md 9.2 asks: below 3 h to 5 min,
// otherwise to the hour.
func etaText(d time.Duration) string {
	step := time.Hour
	if d < 3*time.Hour {
		step = 5 * time.Minute
	}
	return trayicon.ShortDuration(d.Round(step))
}

// enrichToast returns t with the display values the policy does not know:
// event time, forecast, outage duration, power, capacity. outs are the
// station's recent outages (needed for grid_back and grid_flapping only).
// Forecasts come only from a live snapshot. t.Args is not modified.
func enrichToast(t notify.Toast, s monitor.Snapshot, outs []store.Outage) notify.Toast {
	a := make(map[string]string, len(t.Args)+4)
	for k, v := range t.Args {
		a[k] = v
	}
	t.Args = a
	set := func(k, v string) {
		if _, ok := a[k]; !ok && v != "" {
			a[k] = v
		}
	}
	live := s.Link == link.Live
	noLink := t.Kind == notify.NoLink || t.Kind == notify.NoLinkRestored
	// A no-link toast's "time" is the last data (from the policy), not t.At.
	if !t.At.IsZero() && !noLink {
		set("time", hhmm(t.At))
	}
	if noLink {
		// The policy gives a Go duration; shown in the short form.
		if d, err := time.ParseDuration(a["duration"]); err == nil {
			a["duration"] = trayicon.ShortDuration(d)
		}
		if last, err := time.Parse(time.RFC3339, a["lastAt"]); err == nil && !t.At.IsZero() {
			if day := dayTime(last, t.At); day != "" {
				a["time"], a["timeDay"] = day, "1"
			}
		}
	}
	switch t.Kind {
	case notify.GridLost:
		eta := s.F.ToEmpty
		if s.Mode != detect.Discharging {
			eta = s.F.LastsFor
		}
		if live && eta != nil {
			set("lasts", etaText(*eta))
		}
	case notify.GridBack:
		for _, o := range outs {
			if strconv.FormatInt(o.ID, 10) != t.Tag || o.End == nil {
				continue
			}
			if o.DiedAt != nil {
				// The body already says «≈»; the dead station's time lies in a
				// gap, so the whole span is the better estimate here.
				set("duration", trayicon.ShortDuration(o.End.Sub(o.Start)))
				set("diedAt", hhmm(*o.DiedAt))
			} else if b := s.PreviousBounds; b != nil && s.Previous != nil && s.Previous.ID == o.ID {
				// Only the observed time; a lower bound if part of it lies in a gap.
				d := trayicon.ShortDuration(b.Known)
				if b.AtLeast {
					d = i18n.T("duration.atLeast.short", "duration", d)
				}
				set("duration", d)
			} else {
				set("duration", trayicon.ShortDuration(o.End.Sub(o.Start)))
			}
		}
		if live && s.F.FullAt != nil {
			set("fullAt", hhmm(*s.F.FullAt))
		}
	case notify.GridFlapping:
		n := 0
		for _, o := range outs {
			if !o.Start.After(t.At) && t.At.Sub(o.Start) < flapWindow {
				n++
			}
		}
		if n > 0 {
			set("count", strconv.Itoa(n))
		}
	case notify.LowCharge:
		if live && s.Last != nil && s.Last.BatteryW != nil {
			set("watts", trayicon.GroupInt(int64(math.Abs(*s.Last.BatteryW)+0.5)))
		}
		if live && s.F.ToEmpty != nil {
			set("lasts", etaText(*s.F.ToEmpty))
		}
	case notify.Full:
		if wh := s.Profile.NominalWh; wh > 0 {
			set("capacity", trayicon.Decimal(wh/1000))
		}
		if live && s.F.LastsFor != nil {
			set("lasts", etaText(*s.F.LastsFor))
		}
	}
	return t
}

// Notifier sends Windows toasts.
//
// The Wails notification service registers the app with Windows and reports
// clicks. The toasts themselves are built here and shown through the Windows
// Runtime (toastpush.go), because the Wails sender rejects text with emoji and
// cannot tag a toast or keep it from popping up.
type Notifier struct {
	svc      *notifications.NotificationService
	iconOnce sync.Once
	// TitlePrefix goes in front of every toast title. -simulate sets it, so a
	// simulated "grid lost" is never taken for a real one.
	TitlePrefix string
	logos       map[appIcon]string // file URIs; set before the first toast
}

// SetLogoDir writes the toast logos (app icon on / off) into dir; from then
// on every toast shows the one matching its event. Call it before a.Run.
func (n *Notifier) SetLogoDir(dir string) error {
	logos, err := writeToastLogos(dir)
	if err != nil {
		return err
	}
	n.logos = logos
	return nil
}

// logo is the file URI of the toast logo for kind: the "off" icon for the
// toasts about trouble (iconForToast), the "on" one for every other kind and
// for a plain toast (kind ""). "" until SetLogoDir.
func (n *Notifier) logo(k notify.Kind) string {
	return n.logos[iconForToast(k)]
}

// NewNotifier creates the notification service; register Notifier.Service()
// in application.Options.Services.
func NewNotifier() *Notifier {
	return &Notifier{svc: notifications.New()}
}

// Service returns the underlying Wails service for registration.
func (n *Notifier) Service() *notifications.NotificationService {
	return n.svc
}

// NavigateEvent is the Go -> JS event that opens a tab: a toast click
// (flows 2.8) sends where the toast leads.
const NavigateEvent = "navigate"

func init() {
	application.RegisterEvent[Navigate](NavigateEvent)
}

// Navigate names a tab of the window (the frontend View) and, for Outages,
// the outage to highlight; Section is the part of Settings to show.
type Navigate struct {
	View     string
	OutageID int64
	Section  string
}

// ToastRoute is where a click on a toast of kind with tag leads (flows 2.8).
// open is the id of the outage still going on (0 if none). An outage toast
// leads to Overview while its outage lasts and to that outage in Outages once
// it has ended. Only a numeric Tag is an outage id; "link-…" is a
// no-connection episode, which leads to Overview.
func ToastRoute(kind notify.Kind, tag string, open int64) Navigate {
	switch kind {
	case notify.GridLost, notify.GridBack:
		id, err := strconv.ParseInt(tag, 10, 64)
		if err != nil || id <= 0 {
			return Navigate{View: "outages"}
		}
		if kind == notify.GridLost && id == open {
			return Navigate{View: "overview"}
		}
		return Navigate{View: "outages", OutageID: id}
	case notify.GridFlapping:
		return Navigate{View: "outages"}
	case notify.Test:
		return Navigate{View: "settings", Section: "notifications"}
	case notify.Update:
		return Navigate{View: "settings", Section: "about"}
	}
	return Navigate{View: "overview"}
}

// OnClick calls fn with the station id, Kind and Tag of the toast the user
// clicked (all empty for a plain toast).
func (n *Notifier) OnClick(fn func(stationID string, kind notify.Kind, tag string)) {
	n.svc.OnNotificationResponse(func(r notifications.NotificationResult) {
		if r.Error != nil {
			return
		}
		info := r.Response.UserInfo
		id, _ := info["station_id"].(string)
		kind, _ := info["intent"].(string)
		tag, _ := info["event_id"].(string)
		fn(id, notify.Kind(kind), tag)
	})
}

// Toast shows a plain toast with the given title and body.
func (n *Notifier) Toast(id, title, body string) error {
	return n.push(notifications.NotificationOptions{ID: id, Title: title, Body: body}, "", n.logo(""), toastPush{Tag: id})
}

// Deliver shows a monitor toast, with display values from the latest
// snapshot s and, for grid_back / grid_flapping, the stored outages.
func (n *Notifier) Deliver(t notify.Toast, s monitor.Snapshot, st *store.Store) error {
	var outs []store.Outage
	if t.Kind == notify.GridBack || t.Kind == notify.GridFlapping {
		var err error
		outs, err = st.Outages(t.StationID, t.At.Add(-flapWindow), t.At.Add(time.Second))
		if err != nil {
			slog.Warn("toast: outages", "err", err)
		}
	}
	title, body := ToastText(enrichToast(t, s, outs))
	return n.push(notifications.NotificationOptions{
		ID:    toastID(t),
		Title: title,
		Body:  body,
		// Carried back on click (flows 2.8): where the click should lead.
		Data: map[string]interface{}{"station_id": t.StationID, "intent": string(t.Kind), "event_id": toastTag(t)},
	}, attribution(s), n.logo(t.Kind), toastPlace(t))
}

// toastPlace says where the toast goes in the notification centre and
// whether it may pop up. Toasts of one outage share Tag and Group, so "grid
// back" replaces "grid lost" (flows 2.7); the other kinds keep one entry per
// kind and station. A silenced toast (quiet hours, "do not disturb") gets no
// banner — except "low charge", which always pops (flows 5.3).
//
// "connection restored" replaces "no connection" the same way: both carry
// the Tag "link-<unix>", which is kept as is.
func toastPlace(t notify.Toast) toastPush {
	tag := string(t.Kind)
	if tt := toastTag(t); tt != "" {
		tag = taggedID(tt)
	}
	return toastPush{Tag: tag, Group: t.StationID, SuppressPopup: t.Silent && t.Kind != notify.LowCharge}
}

// taggedID: a "link-…" Tag (no-connection episode) is used as is; any other
// Tag is an outage id and gets "outage-".
func taggedID(tag string) string {
	if strings.HasPrefix(tag, "link-") {
		return tag
	}
	return "outage-" + tag
}

// push shows the toast described by o, with the logo image (file URI, may be
// empty), at the given place; p.AppID and p.XML are filled in here.
func (n *Notifier) push(o notifications.NotificationOptions, attribution, logo string, p toastPush) error {
	o.Title = n.TitlePrefix + o.Title
	// The click comes back through the Wails service, which expects its own
	// launch payload.
	payload, err := json.Marshal(notifications.NotificationPayload{
		Action: notifications.DefaultActionIdentifier, Options: o})
	if err != nil {
		return err
	}
	// No banner also means no sound.
	p.XML, err = toastXML(o.Title, o.Body, attribution, logo, base64.StdEncoding.EncodeToString(payload), p.SuppressPopup)
	if err != nil {
		return err
	}
	p.AppID = application.Get().Config().Name
	n.iconOnce.Do(saveToastIcon)
	return pushToast(p)
}

// saveToastIcon writes the app icon where the toast registration (IconUri,
// set by the Wails service) expects it; without the file toasts show a blank
// logo.
func saveToastIcon() {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Classes\AppUserModelId\`+application.Get().Config().Name, registry.QUERY_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	path, _, err := k.GetStringValue("IconUri")
	if err != nil || path == "" {
		return
	}
	if icon, err := application.NewIconFromResource(w32.GetModuleHandle(""), 3); err == nil {
		_ = w32.SaveHIconAsPNG(icon, path)
	}
}

// attribution is the small bottom line naming the station (copy.md 12.3).
func attribution(s monitor.Snapshot) string {
	name, model := s.Station.Name, s.Station.Model
	if model == "" {
		model = s.Profile.Model
	}
	if name == "" && model == "" {
		return appName // no station yet: only the test toast is sent then
	}
	if name == "" || model == "" {
		return name + model
	}
	if long := i18n.T("notify.attribution", "station", name, "model", model); utf8.RuneCountInString(long) <= 40 {
		return long
	}
	return i18n.T("notify.attribution.short", "station", name)
}

type toastDoc struct {
	XMLName        xml.Name   `xml:"toast"`
	ActivationType string     `xml:"activationType,attr"`
	Launch         string     `xml:"launch,attr"`
	Duration       string     `xml:"duration,attr"`
	Binding        toastBind  `xml:"visual>binding"`
	Audio          toastAudio `xml:"audio"`
}

type toastBind struct {
	Template string      `xml:"template,attr"`
	Texts    []toastLine `xml:"text"`
	Image    *toastImage `xml:"image,omitempty"`
}

// toastImage replaces the app logo of the toast (the app icon in the state
// the toast is about).
type toastImage struct {
	Placement string `xml:"placement,attr"`
	Src       string `xml:"src,attr"`
}

type toastLine struct {
	Placement string `xml:"placement,attr,omitempty"`
	MaxLines  string `xml:"hint-maxLines,attr,omitempty"`
	Value     string `xml:",chardata"`
}

type toastAudio struct {
	Src    string `xml:"src,attr,omitempty"`
	Silent string `xml:"silent,attr,omitempty"`
}

// toastXML builds the toast document. logo is the file URI of the logo image
// (none if empty); launch is handed back on click.
func toastXML(title, body, attribution, logo, launch string, silent bool) (string, error) {
	d := toastDoc{ActivationType: "foreground", Launch: launch, Duration: "short",
		Binding: toastBind{Template: "ToastGeneric", Texts: []toastLine{{MaxLines: "1", Value: title}}},
		Audio:   toastAudio{Src: "ms-winsoundevent:Notification.Default"}}
	if body != "" {
		d.Binding.Texts = append(d.Binding.Texts, toastLine{Value: body})
	}
	if attribution != "" {
		d.Binding.Texts = append(d.Binding.Texts, toastLine{Placement: "attribution", Value: attribution})
	}
	if logo != "" {
		d.Binding.Image = &toastImage{Placement: "appLogoOverride", Src: logo}
	}
	if silent {
		d.Audio = toastAudio{Silent: "true"}
	}
	b, err := xml.Marshal(d)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// toastTag is the toast's Tag, or "" when it names no outage: a grid toast
// whose outage the store failed to open carries the id 0 ("0"), and must
// neither share "outage-0" with every other such toast nor lead to it.
func toastTag(t notify.Toast) string {
	if t.Tag == "0" {
		return ""
	}
	return t.Tag
}

// toastID identifies the toast in the click payload: grid_lost and grid_back
// of one outage share it ("outage-<id>"), as do no_link and
// no_link_restored of one episode (their Tag "link-<unix>", kept as is).
func toastID(t notify.Toast) string {
	if tag := toastTag(t); tag != "" {
		return taggedID(tag)
	}
	return string(t.Kind) + "-" + strconv.FormatInt(t.At.Unix(), 10)
}
