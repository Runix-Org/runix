// Package appearanceview delivers the shared active appearance to QML.
package appearanceview

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"runtime"

	"runix/internal/appearance"
	"runix/internal/theme"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/qml"
)

//go:embed Appearance.qml
var document []byte

const (
	snapshotRole = int(qt.UserRole) + 1
	commandRole  = int(qt.UserRole) + 2
)

type themeOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Adapter owns the appearance model; Delete must follow destruction of its QML engine.
type Adapter struct {
	model       *qt.QAbstractListModel
	root        *qt.QModelIndex
	state       *appearance.State
	lightThemes []themeOption
	darkThemes  []themeOption
	stopped     bool
	encoded     string
}

type colorValue struct {
	R float64 `json:"r"`
	G float64 `json:"g"`
	B float64 `json:"b"`
	A float64 `json:"a"`
}

type sampleValue struct {
	Context theme.Context `json:"context"`
	Y       float64       `json:"y"`
	Color   colorValue    `json:"color"`
}

type diagnosticValue struct {
	Minimum float64     `json:"minimum"`
	Target  float64     `json:"target"`
	Passing bool        `json:"passing"`
	Changed bool        `json:"changed"`
	Worst   sampleValue `json:"worst"`
}

type foregroundValue struct {
	Name       string          `json:"name"`
	Original   colorValue      `json:"original"`
	Effective  colorValue      `json:"effective"`
	Contexts   []theme.Context `json:"contexts"`
	Diagnostic diagnosticValue `json:"diagnostic"`
}

type rolesValue struct {
	Original        theme.Colors          `json:"original"`
	ImproveContrast bool                  `json:"improve_contrast"`
	Surfaces        map[string]colorValue `json:"surfaces"`
	Foregrounds     []foregroundValue     `json:"foregrounds"`
}

type payload struct {
	Preferences    appearance.Preferences `json:"preferences"`
	Scheme         appearance.Scheme      `json:"scheme"`
	ActiveID       string                 `json:"active_id"`
	Name           string                 `json:"name"`
	Appearance     theme.Appearance       `json:"appearance"`
	FallbackReason string                 `json:"fallback_reason"`
	Error          string                 `json:"error"`
	Roles          rolesValue             `json:"roles"`
	LightThemes    []themeOption          `json:"light_themes"`
	DarkThemes     []themeOption          `json:"dark_themes"`
}

// Install exposes activeAppearance and appearanceStateModel. Qt.styleHints.colorScheme
// and its colorSchemeChanged notification drive the system scheme directly in QML.
func Install(engine *qml.QQmlApplicationEngine) (*Adapter, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("locate user config directory: %w", err)
	}
	catalog, err := theme.Bundled()
	if err != nil {
		return nil, fmt.Errorf("load theme catalog: %w", err)
	}
	component := qml.NewQQmlComponent2(engine.QQmlEngine)
	component.SetParent(engine.QObject)
	url := qt.NewQUrl3("qrc:/runix/appearanceview/Appearance.qml")
	defer url.Delete()
	component.SetData(document, url)
	if !component.IsReady() {
		return nil, fmt.Errorf("load appearance component: %s", component.ErrorString())
	}
	a := &Adapter{
		model: qt.NewQAbstractListModel(), root: qt.NewQModelIndex(), state: appearance.New(root, catalog),
		lightThemes: themeOptions(catalog.List(theme.Light)), darkThemes: themeOptions(catalog.List(theme.Dark)),
	}
	a.encode()
	a.model.OnRowCount(func(parent *qt.QModelIndex) int {
		if parent.IsValid() {
			return 0
		}
		return 1
	})
	a.model.OnRoleNames(func(_ func() map[int][]byte) map[int][]byte {
		return map[int][]byte{snapshotRole: []byte("snapshot"), commandRole: []byte("command")}
	})
	a.model.OnFlags(func(super func(*qt.QModelIndex) qt.ItemFlag, index *qt.QModelIndex) qt.ItemFlag {
		return super(index) | qt.ItemIsEditable
	})
	a.model.OnData(func(index *qt.QModelIndex, role int) *qt.QVariant {
		if !index.IsValid() || index.Row() != 0 || role != snapshotRole {
			return qt.NewQVariant()
		}
		return qt.NewQVariant14(a.encoded)
	})
	a.model.OnSetData(func(_ func(*qt.QModelIndex, *qt.QVariant, int) bool,
		index *qt.QModelIndex, value *qt.QVariant, role int,
	) bool {
		if a.stopped || !index.IsValid() || index.Row() != 0 || role != commandRole {
			return false
		}
		var command struct {
			Op          string                 `json:"op"`
			Preferences appearance.Preferences `json:"preferences"`
			Scheme      appearance.Scheme      `json:"scheme"`
		}
		if json.Unmarshal([]byte(value.ToString()), &command) != nil {
			return false
		}
		saved := true
		switch command.Op {
		case "preferences":
			saved = a.state.SetPreferences(command.Preferences) == nil
		case "scheme":
			a.state.SetScheme(command.Scheme)
		default:
			return false
		}
		a.publish()
		return saved
	})
	engine.RootContext().SetContextProperty("appearanceStateModel", a.model.QObject)
	object := component.Create(engine.RootContext())
	if object == nil {
		a.Delete()
		return nil, fmt.Errorf("create active appearance object: see Qt errors above")
	}
	object.SetParent(engine.QObject)
	engine.RootContext().SetContextProperty("activeAppearance", object)
	return a, nil
}

func (a *Adapter) snapshot() payload {
	s := a.state.Snapshot()
	roles := rolesValue{
		Original: s.Roles.Original, ImproveContrast: s.Roles.ImproveContrast,
		Surfaces:    make(map[string]colorValue, 30),
		Foregrounds: make([]foregroundValue, 0, len(s.Roles.Foregrounds)),
	}
	surfaces := s.Roles.Surfaces
	for name, color := range map[string]theme.Color{
		"b0": surfaces.B0, "b1": surfaces.B1, "selected": surfaces.Selected,
		"hover": surfaces.Hover, "press": surfaces.Press,
		"field": surfaces.Field, "button": surfaces.Button, "separator": surfaces.Separator, "border": surfaces.Border,
		"hover_border": surfaces.HoverBorder, "disabled_border": surfaces.DisabledBorder,
		"disabled_track": surfaces.DisabledTrack,
		"track":          surfaces.Track, "scrollbar": surfaces.Scrollbar, "scrollbar_hover": surfaces.ScrollbarHover,
		"scrollbar_pressed": surfaces.ScrollbarPressed, "scrim": surfaces.Scrim,
		"selection": surfaces.Selection, "checked": surfaces.Checked,
	} {
		roles.Surfaces[name] = colorJSON(color)
	}
	for i, color := range surfaces.Tags {
		roles.Surfaces[fmt.Sprintf("tag_%d", i)] = colorJSON(color)
	}
	for _, foreground := range s.Roles.Foregrounds {
		d := foreground.Diagnostic
		roles.Foregrounds = append(roles.Foregrounds, foregroundValue{
			Name: foreground.Name, Original: colorJSON(foreground.Original),
			Effective: colorJSON(foreground.Effective), Contexts: foreground.Contexts,
			Diagnostic: diagnosticValue{
				Minimum: d.Minimum, Target: d.Target, Passing: d.Passing, Changed: d.Changed,
				Worst: sampleValue{Context: d.Worst.Context, Y: d.Worst.Y, Color: colorJSON(d.Worst.Color)},
			},
		})
	}
	return payload{
		Preferences: s.Preferences, Scheme: s.Scheme, ActiveID: s.ActiveID, Name: s.Name,
		Appearance: s.Appearance, FallbackReason: s.FallbackReason, Error: s.Error, Roles: roles,
		LightThemes: a.lightThemes, DarkThemes: a.darkThemes,
	}
}

func colorJSON(c theme.Color) colorValue { return colorValue{R: c.R, G: c.G, B: c.B, A: c.A} }

func themeOptions(entries []theme.Entry) []themeOption {
	options := make([]themeOption, 0, len(entries))
	for _, entry := range entries {
		options = append(options, themeOption{ID: entry.ID, Name: entry.Name})
	}
	return options
}

func (a *Adapter) encode() {
	data, _ := json.Marshal(a.snapshot())
	a.encoded = string(data)
}

func (a *Adapter) publish() {
	a.encode()
	index := a.model.Index(0, 0, a.root)
	runtime.SetFinalizer(index, nil)
	defer index.Delete()
	a.model.DataChanged(index, index)
}

// Delete releases Qt model resources after the engine and its consumers are gone.
func (a *Adapter) Delete() {
	a.stopped = true
	runtime.SetFinalizer(a.model, nil)
	runtime.SetFinalizer(a.root, nil)
	a.model.Delete()
	a.root.Delete()
}
