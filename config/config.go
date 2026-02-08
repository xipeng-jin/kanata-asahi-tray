package config

import (
	"bytes"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/elliotchance/orderedmap/v2"
	"github.com/k0kubun/pp/v3"
	"github.com/kr/pretty"
	"github.com/labstack/gommon/log"
	"github.com/pelletier/go-toml/v2"
	tomlu "github.com/pelletier/go-toml/v2/unstable"

	_ "embed"
)

//go:embed default_config.toml
var defaultConfigContent string

type Config struct {
	PresetDefaults Preset
	General        GeneralConfigOptions
	Presets        *OrderedMap[string, *Preset]
}

type Preset struct {
	Autorun             bool
	KanataExecutable    string
	KanataConfig        string
	TcpPort             int
	LayerIcons          map[string]string
	Hooks               Hooks
	TrackpadWhileTyping TrackpadWhileTyping
	ExtraArgs           []string
	AutorestartOnCrash  bool
}

type TrackpadWhileTyping struct {
	Enabled        bool
	KeyboardDevice string
	PointerDevice  string
	TriggerKey     string
}

func (m *Preset) GoString() string {
	pp.Default.SetColoringEnabled(false)
	return pp.Sprintf("%s", m)
}

type GeneralConfigOptions struct {
	AllowConcurrentPresets bool
	ControlServerEnable    bool
	ControlServerPort      int
}

// Parsed hooks that contain list of args.
type Hooks struct {
	PreStart       [][]string
	PostStart      [][]string
	PostStartAsync [][]string
	PostStop       [][]string
}

// =========
// All golang toml parsers suck :/

type config struct {
	PresetDefaults *preset               `toml:"defaults"`
	General        *generalConfigOptions `toml:"general"`
	Presets        map[string]preset     `toml:"presets"`
}

type preset struct {
	Autorun             *bool                `toml:"autorun"`
	KanataExecutable    *string              `toml:"kanata_executable"`
	KanataConfig        *string              `toml:"kanata_config"`
	TcpPort             *int                 `toml:"tcp_port"`
	LayerIcons          map[string]string    `toml:"layer_icons"`
	Hooks               *hooks               `toml:"hooks"`
	TrackpadWhileTyping *trackpadWhileTyping `toml:"trackpad_while_typing"`
	ExtraArgs           extraArgs            `toml:"extra_args"`
	AutorestartOnCrash  *bool                `toml:"autorestart_on_crash"`
}

func (p *preset) applyDefaults(defaults *preset) {
	if p.Autorun == nil {
		p.Autorun = defaults.Autorun
	}
	if p.KanataExecutable == nil {
		p.KanataExecutable = defaults.KanataExecutable
	}
	if p.KanataConfig == nil {
		p.KanataConfig = defaults.KanataConfig
	}
	if p.TcpPort == nil {
		p.TcpPort = defaults.TcpPort
	}
	//// Excluding layer icons is intended because they are handled specially.
	//
	// if p.LayerIcons == nil {
	// 	p.LayerIcons = defaults.LayerIcons
	// }
	if p.Hooks == nil {
		p.Hooks = defaults.Hooks
	}
	if p.TrackpadWhileTyping == nil {
		p.TrackpadWhileTyping = defaults.TrackpadWhileTyping
	} else {
		p.TrackpadWhileTyping.applyDefaults(defaults.TrackpadWhileTyping)
	}
	if p.ExtraArgs == nil {
		p.ExtraArgs = defaults.ExtraArgs
	}
	if p.AutorestartOnCrash == nil {
		p.AutorestartOnCrash = defaults.AutorestartOnCrash
	}
}

func (p *preset) intoExported() (*Preset, error) {
	result := &Preset{TrackpadWhileTyping: defaultTrackpadWhileTyping()}
	if p.Autorun != nil {
		result.Autorun = *p.Autorun
	}
	if p.KanataExecutable != nil {
		result.KanataExecutable = *p.KanataExecutable
	}
	if p.KanataConfig != nil {
		result.KanataConfig = *p.KanataConfig
	}
	if p.TcpPort != nil {
		result.TcpPort = *p.TcpPort
	}
	if p.LayerIcons != nil {
		result.LayerIcons = p.LayerIcons
	}
	if p.Hooks != nil {
		x, err := p.Hooks.intoExported()
		if err != nil {
			return nil, err
		}
		result.Hooks = *x
	}
	if p.TrackpadWhileTyping != nil {
		x, err := p.TrackpadWhileTyping.intoExported()
		if err != nil {
			return nil, err
		}
		result.TrackpadWhileTyping = *x
	}
	if p.ExtraArgs != nil {
		x, err := p.ExtraArgs.intoExported()
		if err != nil {
			return nil, err
		}
		result.ExtraArgs = x
	}
	if p.AutorestartOnCrash != nil {
		result.AutorestartOnCrash = *p.AutorestartOnCrash
	}
	return result, nil
}

type generalConfigOptions struct {
	AllowConcurrentPresets *bool `toml:"allow_concurrent_presets"`
	ControlServerEnable    *bool `toml:"control_server_enable"`
	ControlServerPort      *int  `toml:"control_server_port"`
}

type trackpadWhileTyping struct {
	Enabled        *bool   `toml:"enabled"`
	KeyboardDevice *string `toml:"keyboard_device"`
	PointerDevice  *string `toml:"pointer_device"`
	TriggerKey     *string `toml:"trigger_key"`
}

func defaultTrackpadWhileTyping() TrackpadWhileTyping {
	return TrackpadWhileTyping{
		Enabled:        false,
		KeyboardDevice: "auto:kanata",
		PointerDevice:  "auto:touchpad",
		TriggerKey:     "KEY_FN",
	}
}

func (t *trackpadWhileTyping) applyDefaults(defaults *trackpadWhileTyping) {
	if defaults == nil {
		return
	}
	if t.Enabled == nil {
		t.Enabled = defaults.Enabled
	}
	if t.KeyboardDevice == nil {
		t.KeyboardDevice = defaults.KeyboardDevice
	}
	if t.PointerDevice == nil {
		t.PointerDevice = defaults.PointerDevice
	}
	if t.TriggerKey == nil {
		t.TriggerKey = defaults.TriggerKey
	}
}

func (t *trackpadWhileTyping) intoExported() (*TrackpadWhileTyping, error) {
	result := defaultTrackpadWhileTyping()

	if t.Enabled != nil {
		result.Enabled = *t.Enabled
	}
	if t.KeyboardDevice != nil {
		v := strings.TrimSpace(*t.KeyboardDevice)
		if v != "" {
			result.KeyboardDevice = v
		}
	}
	if t.PointerDevice != nil {
		v := strings.TrimSpace(*t.PointerDevice)
		if v != "" {
			result.PointerDevice = v
		}
	}
	if t.TriggerKey != nil {
		v := strings.TrimSpace(*t.TriggerKey)
		if v != "" {
			result.TriggerKey = strings.ToUpper(v)
		}
	}

	if strings.EqualFold(result.KeyboardDevice, "auto:kanata") {
		result.KeyboardDevice = "auto:kanata"
	}
	if strings.EqualFold(result.PointerDevice, "auto:touchpad") {
		result.PointerDevice = "auto:touchpad"
	}

	if strings.HasPrefix(strings.ToLower(result.KeyboardDevice), "auto:") && result.KeyboardDevice != "auto:kanata" {
		return nil, fmt.Errorf("invalid trackpad_while_typing.keyboard_device '%s': allowed auto selector is auto:kanata or a /dev/input/eventX path", result.KeyboardDevice)
	}
	if strings.HasPrefix(strings.ToLower(result.PointerDevice), "auto:") && result.PointerDevice != "auto:touchpad" {
		return nil, fmt.Errorf("invalid trackpad_while_typing.pointer_device '%s': allowed auto selector is auto:touchpad or an exact Hyprland device name", result.PointerDevice)
	}
	if result.TriggerKey != "KEY_FN" && result.TriggerKey != "KEY_LEFTALT" && result.TriggerKey != "KEY_RIGHTALT" {
		return nil, fmt.Errorf("invalid trackpad_while_typing.trigger_key '%s': allowed values are KEY_FN, KEY_LEFTALT, KEY_RIGHTALT", result.TriggerKey)
	}

	return &result, nil
}

type hooks struct {
	CmdTemplate    []string `toml:"cmd_template"`
	PreStart       []string `toml:"pre-start"` // TODO: rename to snake case.
	PostStart      []string `toml:"post-start"`
	PostStartAsync []string `toml:"post-start-async"`
	PostStop       []string `toml:"post-stop"`
}

type cmdTempl struct {
	inner [](func(s string) string)
}

func newCmdTemplFromRaw(cmdTemplate []string) (*cmdTempl, error) {
	var r = new(cmdTempl)
	var fmtSeqCount int

	for _, arg := range cmdTemplate {
		fmtSeqCount += strings.Count(arg, "{}")
		arg := arg
		r.inner = append(r.inner, func(s string) string {
			return strings.ReplaceAll(arg, "{}", s)
		})
	}

	if fmtSeqCount != 1 {
		return nil, fmt.Errorf(
			"expected exactly one occurence of {}, found %d",
			fmtSeqCount,
		)
	}

	return r, nil
}

func (t *cmdTempl) apply(s string) (finalArgv []string) {
	for _, fn := range t.inner {
		finalArgv = append(finalArgv, fn(s))
	}
	return finalArgv
}

func (t *cmdTempl) applyMany(xs []string) [][]string {
	var results [][]string
	for _, x := range xs {
		results = append(results, t.apply(x))
	}
	return results
}

func (p *hooks) intoExported() (*Hooks, error) {
	cmdTemplate := p.CmdTemplate
	if cmdTemplate == nil {
		if runtime.GOOS == "windows" {
			cmdTemplate = []string{"{}"} // TODO: better default? maybe powershell?
		} else {
			cmdTemplate = []string{"/bin/sh", "-c", "{}"}
		}
	}
	templ, err := newCmdTemplFromRaw(cmdTemplate)
	if err != nil {
		return nil, fmt.Errorf("parsing cmd_template: %w", err)
	}
	return &Hooks{
		PreStart:       templ.applyMany(p.PreStart),
		PostStart:      templ.applyMany(p.PostStart),
		PostStartAsync: templ.applyMany(p.PostStartAsync),
		PostStop:       templ.applyMany(p.PostStop),
	}, nil
}

type extraArgs []string

func (e extraArgs) intoExported() ([]string, error) {
	for _, s := range e {
		if strings.HasPrefix(s, "--port") || strings.HasPrefix(s, "-p") {
			return nil, fmt.Errorf("port argument is not allowed in extra_args, use tcp_port instead")
		}
	}
	return e, nil
}

func ReadConfigOrCreateIfNotExist(configFilePath string) (*Config, error) {
	var cfg *config = &config{}
	// Golang map don't keep track of insertion order, so we need to get the
	// order of declarations in toml separately.
	layersNames, err := layersOrder([]byte(defaultConfigContent))
	if err != nil {
		panic(fmt.Errorf("default config failed layersOrder: %v", err))
	}
	err = toml.Unmarshal([]byte(defaultConfigContent), &cfg)
	if err != nil {
		panic(fmt.Errorf("failed to parse default config: %v", err))
	}
	// temporarily remove default presets
	presetsFromDefaultConfig := cfg.Presets
	cfg.Presets = nil

	// Does the file not exist?
	if _, err := os.Stat(configFilePath); os.IsNotExist(err) {
		log.Infof("Config file doesn't exist. Creating default config. Path: '%s'", configFilePath)
		err = os.WriteFile(configFilePath, []byte(defaultConfigContent), os.FileMode(0600))
		if err != nil {
			return nil, fmt.Errorf("failed to write default config file to '%s': %v", configFilePath, err)
		}
	} else {
		// Load the existing file.
		content, err := os.ReadFile(configFilePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read file '%s': %v", configFilePath, err)
		}
		err = toml.NewDecoder(bytes.NewReader(content)).Decode(&cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to parse config file '%s': %v", configFilePath, err)
		}
		lnames, err := layersOrder(content)
		if err != nil {
			panic("default config failed layersOrder")
		}
		if len(lnames) != 0 {
			layersNames = lnames
		}
	}

	if cfg.Presets == nil {
		cfg.Presets = presetsFromDefaultConfig
	}

	defaults := cfg.PresetDefaults

	defaultsExported, err := defaults.intoExported()
	if err != nil {
		return nil, err
	}
	var cfg2 *Config = &Config{
		PresetDefaults: *defaultsExported,
		General: GeneralConfigOptions{
			AllowConcurrentPresets: *cfg.General.AllowConcurrentPresets,
			ControlServerEnable:    *cfg.General.ControlServerEnable,
			ControlServerPort:      *cfg.General.ControlServerPort,
		},
		Presets: NewOrderedMap[string, *Preset](),
	}

	for _, layerName := range layersNames {
		v, ok := cfg.Presets[layerName]
		if !ok {
			panic("layer names should match")
		}
		v.applyDefaults(defaults)
		exported, err := v.intoExported()
		if err != nil {
			return nil, err
		}
		cfg2.Presets.Set(layerName, exported)
	}

	log.Debugf("loaded config: %s", pretty.Sprint(cfg2))
	return cfg2, nil
}

// Returns an array of layer names from config in order of declaration.
func layersOrder(cfgContent []byte) ([]string, error) {
	layerNamesInOrder := []string{}

	p := tomlu.Parser{}
	p.Reset([]byte(cfgContent))

	// iterate over all top level expressions
	for p.NextExpression() {
		e := p.Expression()

		if e.Kind != tomlu.Table {
			continue
		}

		// Let's look at the key. It's an iterator over the multiple dotted parts of the key.
		it := e.Key()
		parts := keyAsStrings(it)

		// we're only considering keys that look like `presets.XXX`
		if len(parts) != 2 {
			continue
		}
		if parts[0] != "presets" {
			continue
		}

		layerNamesInOrder = append(layerNamesInOrder, string(parts[1]))
	}

	return layerNamesInOrder, nil
}

// helper to transfor a key iterator to a slice of strings
func keyAsStrings(it tomlu.Iterator) []string {
	var parts []string
	for it.Next() {
		n := it.Node()
		parts = append(parts, string(n.Data))
	}
	return parts
}

type OrderedMap[K string, V fmt.GoStringer] struct {
	*orderedmap.OrderedMap[K, V]
}

func NewOrderedMap[K string, V fmt.GoStringer]() *OrderedMap[K, V] {
	return &OrderedMap[K, V]{
		OrderedMap: orderedmap.NewOrderedMap[K, V](),
	}
}

// impl `fmt.GoStringer`
func (m *OrderedMap[K, V]) GoString() string {
	indent := "    "
	keys := []K{}
	values := []V{}
	for it := m.Front(); it != nil; it = it.Next() {
		keys = append(keys, it.Key)
		values = append(values, it.Value)
	}
	builder := strings.Builder{}
	builder.WriteString("{")
	for i := range keys {
		key := keys[i]
		value := values[i]
		valueLines := strings.Split(value.GoString(), "\n")
		for i, vl := range valueLines {
			if i == 0 {
				continue
			}
			valueLines[i] = fmt.Sprintf("%s%s", indent, vl)
		}
		indentedVal := strings.Join(valueLines, "\n")
		builder.WriteString(fmt.Sprintf("\n%s\"%s\": %s", indent, key, indentedVal))
	}
	builder.WriteString("\n}")
	return builder.String()
}
