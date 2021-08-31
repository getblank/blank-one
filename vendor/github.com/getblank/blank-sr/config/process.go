package config

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"sort"
	"time"

	log "github.com/sirupsen/logrus"
)

var (
	// mustacheRgx    = regexp.MustCompile(`(?U)({{.+}})`)
	// handleBarseRgx = regexp.MustCompile(`{?{{\s*(\w*)\s?(\w*)?\s?.*}}`)
	// itemPropsRgx   = regexp.MustCompile(`\$ites.([A-Za-z][A-Za-z0-9]*)`)
	// actionIDRgx    = regexp.MustCompile(`^[A-Za-z_]+[A-Za-z0-9_]*$`)

	storeUpdateHandlers = []func(map[string]Store){}
	configProviders     = []ConfigProvider{}
)

// ConfigProvider is an interface describes config provider
type ConfigProvider interface {
	Name() string
	Get() (map[string]Store, error)
}

// RegisterConfigProvider saves provided config provider to use it later.
func RegisterConfigProvider(p ConfigProvider) {
	confLocker.Lock()
	defer confLocker.Unlock()

	configProviders = append(configProviders, p)
}

func Init(confFile string) {
	makeDefaultSettings()
	readConfig(confFile)
	updated(config)
}

func ReloadConfig(conf map[string]Store) {
	log.Info("Starting to reload config")

	encoded, err := json.Marshal(conf)
	if err != nil {
		log.Errorf("Can't marshal config when reloding: %s", err.Error())
	} else {
		err = ioutil.WriteFile("config.json", encoded, 0755)
		if err != nil {
			log.Errorf("Can't save new config.json: %s", err.Error())
		} else {
			log.Info("New config.json file saved")
		}
	}

	loadFromProviders(conf)
	loadCommonSettings(conf)
	loadServerSettings(conf)
	confLocker.Lock()
	config = Validate(conf)
	confLocker.Unlock()
	updated(config)
}

func OnUpdate(fn func(map[string]Store)) {
	storeUpdateHandlers = append(storeUpdateHandlers, fn)
}

func loadFromProviders(conf map[string]Store) {
	confLocker.Lock()
	defer confLocker.Unlock()

	for _, p := range configProviders {
		cfg, err := p.Get()
		if err != nil {
			log.Warnf("Can't load config from provider %q, error: %s", p.Name(), err)
			continue
		}

		for storeName, storeDesc := range cfg {
			if s, ok := conf[storeName]; ok {
				s.mergeWith(storeDesc)

				conf[storeName] = s
				continue
			}

			conf[storeName] = storeDesc
		}
	}
}

func updated(config map[string]Store) {
	for _, fn := range storeUpdateHandlers {
		fn(config)
	}
}

func readConfig(confFile string) {
	log.Info("Try to load config from: " + confFile)
	file, err := ioutil.ReadFile(confFile)
	if err != nil {
		log.Errorf("Config file read error: %v", err.Error())
		return
	}

	var conf map[string]Store
	err = json.Unmarshal(file, &conf)
	if err != nil {
		log.Error("Error when read objects config", err.Error())
		if v := os.Getenv("NODE_ENV"); v == "DEV" {
			return
		}

		time.Sleep(time.Microsecond * 200)
		os.Exit(1)
	}

	loadFromProviders(conf)
	loadCommonSettings(conf)
	loadServerSettings(conf)
	confLocker.Lock()
	config = Validate(conf)
	confLocker.Unlock()
}

func loadCommonSettings(conf map[string]Store) {
	confLocker.Lock()
	defer confLocker.Unlock()
	cs, ok := conf[ObjCommonSettings]
	if !ok {
		log.Warn("No common settings in config")
		return
	}
	encoded, err := json.Marshal(cs.Entries)
	if err != nil {
		log.Error("Can't marshal common settings", cs.Entries, err.Error())
	} else {
		err = json.Unmarshal(encoded, commonSettings)
		if err != nil {
			log.Error("Can't unmarshal common settings", string(encoded), err.Error())
		}
	}
	encoded, err = json.Marshal(cs.I18n)
	if err != nil {
		log.Error("Can't marshal common i18n", cs.I18n, err.Error())
		return
	}
	err = json.Unmarshal(encoded, &commonSettings.I18n)
	if err != nil {
		log.Error("Can't unmarshal common i18n", string(encoded), err.Error())
	}
}

func loadServerSettings(conf map[string]Store) {
	confLocker.Lock()
	defer confLocker.Unlock()
	ss, ok := conf[ObjServerSettings]
	if !ok {
		log.Warn("No server settings in config")
		return
	}
	encoded, err := json.Marshal(ss.Entries)
	if err != nil {
		log.Error("Can't marshal server settings", ss.Entries, err.Error())
		return
	}
	err = json.Unmarshal(encoded, serverSettings)
	if err != nil {
		log.Error("Can't unmarshal server settings", string(encoded), err.Error())
	}
	serverSettings.jwtTTL = nil
}

func Validate(conf map[string]Store) map[string]Store {
	// confLocker.Lock()
	// defer confLocker.Unlock()
	_conf := map[string]Store{}
	var err error

	for store, o := range conf {
		log.Info("Parsing config for store:", store)
		o.Store = store
		if o.Props == nil {
			o.Props = map[string]Prop{}
		}

		// Checking object type
		switch o.Type {
		case ObjDirectory:
			//			log.Info("Store is 'directory' type")
		case ObjProcess:
			//			log.Info("Store is 'process' type")
		case ObjMap:
			//			log.Info("Store is 'inConfigSet' type")
			o.Props = nil
		case ObjWorkspace:
			//			log.Info("Store is 'workspace' type")
			o.Props = nil
		case ObjCampaign:
			//			log.Info("Store is 'campaign' type")
		case ObjNotification:
			//			log.Info("Store is 'notification' type")
		case ObjSingle:
			o.Props["_id"] = Prop{Type: PropString, Display: "none"}
			//			log.Info("Store is 'single' type")
		case ObjFile:
			// 		log.Info("Store is 'file' type")
		case ObjProxy:
			// 		log.Info("Store is 'proxy' type")
		default:
			o.Type = ObjDirectory
		}

		allPropsValid := true

		err = o.validateProps(o.Props, true)
		if err != nil {
			log.Error("Validating props failed:", err)
			allPropsValid = false
			continue
		}

		if allPropsValid {
			_conf[store] = o
		} else {
			log.Error("Invalid Store", store, o)
		}
	}

	config := map[string]Store{}
ConfLoop:
	for storeName := range _conf {
		store := _conf[storeName]
		for name, p := range store.Props {
			if p.Type == PropRef || p.Type == PropRefList || p.Type == PropVirtualRefList {
				_, ok := _conf[p.Store]
				if !ok {
					log.Error("Oppostite store '" + p.Store + "' not exists for prop '" + name + "' in store '" + storeName + "'. Store will ignored!")
					continue ConfLoop
				}
			}

			for subName, subP := range p.Props {
				if subP.Type == PropRef || subP.Type == PropRefList || subP.Type == PropVirtualRefList {
					_, ok := _conf[subP.Store]
					if !ok {
						log.Error("Oppostite store '" + subP.Store + "' not exists for prop '" + name + "." + subName + "' in store '" + storeName + "'. Store will ignored!")
						continue ConfLoop
					}
				}
			}
		}

		switch storeName {
		case DefaultDirectory, DefaultSingle, DefaultCampaign, DefaultNotification, DefaultProcess:
			//			log.Info("This is", store, "store")
		default:
			if defaultDirectory, ok := _conf[DefaultDirectory]; ok {
				store.mergeFilters(&defaultDirectory)
				for _pName, _prop := range defaultDirectory.Props {
					store.LoadDefaultIntoProp(_pName, _prop)
				}
			}
			switch store.Type {
			case ObjProcess:
				if defaultProcess, ok := _conf[DefaultProcess]; ok {
					store.mergeFilters(&defaultProcess)
					for _pName, _prop := range defaultProcess.Props {
						store.LoadDefaultIntoProp(_pName, _prop)
					}
				}
			case ObjNotification:
				if defaultNotification, ok := _conf[DefaultNotification]; ok {
					store.mergeAccess(&defaultNotification)
					for _pName, _prop := range defaultNotification.Props {
						store.LoadDefaultIntoProp(_pName, _prop)
					}
				}
			case ObjSingle:
				if defaultSingle, ok := _conf[DefaultSingle]; ok {
					store.mergeFilters(&defaultSingle)
					for _pName, _prop := range defaultSingle.Props {
						store.LoadDefaultIntoProp(_pName, _prop)
					}
				}
			}
		}

		if len(store.StoreLifeCycle.Migration) > 0 {
			sort.Slice(store.StoreLifeCycle.Migration, func(i, j int) bool {
				return store.StoreLifeCycle.Migration[i].Version < store.StoreLifeCycle.Migration[j].Version
			})
			prevVer := -99
			for i := len(store.StoreLifeCycle.Migration) - 1; i >= 0; i-- {
				v := store.StoreLifeCycle.Migration[i]
				var badMigration bool
				if len(v.Script) == 0 {
					log.Warnf(`Store "%s" migration script for version %d is empty`, storeName, v.Version)
					badMigration = true
				}

				if v.Version == prevVer {
					log.Warnf(`Store "%s" migration script for version %d duplicated`, storeName, v.Version)
					badMigration = true
				}
				prevVer = v.Version

				if badMigration {
					log.Warnf(`Store "%s" migration script for version %d will be ignored due issues described before`, storeName, v.Version)
					store.StoreLifeCycle.Migration = append(store.StoreLifeCycle.Migration[:i], store.StoreLifeCycle.Migration[i+1:]...)
				}
			}
		}
		config[store.Store] = store
	}

	return config
}

func (s *Store) validateProps(props map[string]Prop, parseObjects bool) error {
	for pName, prop := range props {
		prop.Name = pName
		// Processing Type
		if prop.Type == "" {
			prop.Type = PropString
		}

		switch prop.Type {
		case PropWidget, PropAction, PropFile, PropFileList, PropPassword:
			continue
		case PropAny:
			continue
		case PropInt:
			prop.clearStringParams()
			prop.clearRefParams()
			prop.clearObjectParams()
			if prop.Default != nil {
				if d, ok := prop.Default.(map[string]interface{}); ok {
					if d["$expression"] == nil {
						return errors.New("Invalid default int value in prop: '" + pName + "'")
					}
				} else if _, ok := prop.checkDefaultInt(); !ok {
					return errors.New("Invalid default int value in prop: '" + pName + "'")
				}
			}

			_, _, ok := prop.checkMinMaxParams()
			if !ok {
				return errors.New("Wrong min-max params in prop: '" + pName + "'")
			}
			//			if prop.Values != nil && len(prop.Values) > 0 {
			//				for _, v := range prop.Values {
			//					if _, ok := v.Value.(float64); !ok {
			//						return errors.New("Invalid int value in list in prop: '" + pName + "'")
			//					}
			//				}
			//			}
			props[pName] = prop
		case PropFloat:
			prop.clearStringParams()
			prop.clearRefParams()
			prop.clearObjectParams()
			if prop.Default != nil {
				if d, ok := prop.Default.(map[string]interface{}); ok {
					if d["$expression"] == nil {
						return errors.New("Invalid default float value in prop: '" + pName + "'")
					}
				} else if _, ok := prop.checkDefaultFloat(); !ok {
					return errors.New("Invalid default float value in prop: '" + pName + "'")
				}
			}

			_, _, ok := prop.checkMinMaxParams()
			if !ok {
				return errors.New("Wrong min-max params in prop: '" + pName + "'")
			}

			props[pName] = prop
		case PropBool:
			prop.clearStringParams()
			prop.clearRefParams()
			prop.clearNumberParams()
			prop.clearObjectParams()
			if prop.Default != nil {
				if d, ok := prop.Default.(map[string]interface{}); ok {
					if d["$expression"] == nil {
						return errors.New("Invalid default bool value in prop: '" + pName + "'")
					}
				} else if _, ok := prop.Default.(bool); !ok {
					return errors.New("Invalid default bool value in prop: '" + pName + "'")
				}
			}

			props[pName] = prop
		case PropString, PropUUID:
			prop.clearNumberParams()
			prop.clearRefParams()
			prop.clearObjectParams()
			if prop.Default != nil {
				if d, ok := prop.Default.(map[string]interface{}); ok {
					if d["$expression"] == nil {
						return errors.New("Invalid default string value in prop: '" + pName + "'")
					}
				} else if _, ok := prop.Default.(string); !ok {
					return errors.New("Invalid default string value in prop: '" + pName + "'")
				}
			}

			if prop.MinLength < 0 || prop.MaxLength < 0 {
				return errors.New("Wrong minLength or maxLength values in prop: '" + pName + "'")
			}

			if prop.MinLength != 0 && prop.MaxLength != 0 {
				if prop.MinLength > prop.MaxLength {
					return errors.New("minLength > maxLength in prop: '" + pName + "'")
				}
			}

			props[pName] = prop
		case PropDate, PropDateOnly:
			prop.clearStringParams()
			prop.clearRefParams()
			prop.clearObjectParams()
			if prop.Default != nil {
				if d, ok := prop.Default.(map[string]interface{}); ok {
					if d["$expression"] == nil {
						return errors.New("Invalid default date value in prop: '" + pName + "'")
					}
				} else if _, ok := prop.Default.(time.Time); !ok {
					return errors.New("Invalid default date in prop: '" + pName + "'")
				}
			}

			props[pName] = prop
		case PropRef:
			prop.clearStringParams()
			prop.clearNumberParams()
			if prop.Default != nil {
				if d, ok := prop.Default.(map[string]interface{}); ok {
					if d["$expression"] == nil {
						return errors.New("Invalid default ref value in prop: '" + pName + "'")
					}
				} else if _, ok := prop.Default.(string); !ok {
					return errors.New("Invalid default value for ref type in prop: '" + pName + "'")
				}
			}

			if prop.Store == "" {
				return errors.New("Store not provided for ref type in prop: '" + pName + "'")
			}

			props[pName] = prop
		case PropRefList:
			prop.clearStringParams()
			prop.clearNumberParams()
			if prop.Default != nil {
				if d, ok := prop.Default.(map[string]interface{}); ok {
					if d["$expression"] == nil {
						return errors.New("Invalid default refList value in prop: '" + pName + "'")
					}
				} else if _, ok := prop.Default.([]interface{}); !ok {
					return errors.New("Invalid default value for refList type in prop: '" + pName + "'")
				}
			}

			if prop.Store == "" {
				return errors.New("Store not provided for refList type in prop: '" + pName + "'")
			}

			props[pName] = prop
		case PropVirtual:
			prop.clearStringParams()
			prop.clearRefParams()
			prop.clearNumberParams()
			prop.clearObjectParams()
			prop.Default = nil
			props[pName] = prop
		case PropObject:
			if !parseObjects {
				return errors.New("Recursive objects not allowed '" + pName + "'")
			}

			prop.clearStringParams()
			prop.clearRefParams()
			prop.clearNumberParams()
			err := s.validateProps(prop.Props, false)
			if err != nil {
				return err
			}

			props[pName] = prop
		case PropObjectList:
			if !parseObjects {
				return errors.New("Recursive objects not allowed '" + pName + "'")
			}

			prop.Pattern = ""
			prop.Mask = ""
			prop.clearRefParams()
			prop.clearNumberParams()
			err := s.validateProps(prop.Props, false)
			if err != nil {
				return err
			}

			props[pName] = prop
		case PropVirtualRefList:
			prop.clearStringParams()
			prop.clearNumberParams()
			prop.clearObjectParams()
			prop.Default = nil
			if prop.Store == "" {
				return errors.New("Store is not provided for virtualRefList type in prop: '" + pName + "'")
			}

			if prop.ForeignKey == "" && prop.Query == nil {
				return errors.New("Foregn key or query is not provided for virtualRefList type in prop: '" + pName + "'")
			}

			props[pName] = prop
		case PropComments:
			prop.clearStringParams()
			prop.clearNumberParams()
			props[pName] = prop
		case PropVirtualClient:
		default:
			return errors.New("Unknown prop type: '" + pName + "' '" + prop.Type + "'")
		}
	}
	return nil
}

func (p *Prop) checkDefaultFloat() (float64, bool) {
	_def, ok := p.Default.(float64)
	if !ok {
		return 0, false
	}
	return _def, true
}

func (p *Prop) checkDefaultInt() (int, bool) {
	_def, ok := p.checkDefaultFloat()
	if !ok {
		return 0, false
	}
	def := int(_def)
	return def, true
}

func (p *Prop) checkMinMaxParams() (float64, float64, bool) {
	var min, max float64
	if p.Min != nil {
		var ok bool
		min, ok = p.Min.(float64)
		if !ok {
			return 0, 0, false
		}
	}
	if p.Max != nil {
		var ok bool
		max, ok = p.Max.(float64)
		if !ok {
			return 0, 0, false
		}
	}
	if min == 0 && max == 0 {
		return min, max, true
	}
	if min > max {
		return 0, 0, false
	}
	return min, max, true
}

func (p *Prop) clearNumberParams() {
	p.Min = nil
	p.Max = nil
}

func (p *Prop) clearObjectParams() {
	p.Props = nil
}

func (p *Prop) clearRefParams() {
	p.Store = ""
	p.OppositeProp = ""
	p.ExtraQuery = nil
	p.Query = nil
}

func (p *Prop) clearStringParams() {
	p.MinLength = 0
	p.MaxLength = 0
	p.Pattern = ""
	p.Mask = ""
}

func (s *Store) LoadDefaultIntoProp(name string, p Prop) {
	if s.Props == nil {
		s.Props = map[string]Prop{}
	}
	if !p.Configurable {
		s.Props[name] = p
		return
	}

	prop, ok := s.Props[name]
	if !ok {
		s.Props[name] = p
		return
	}

	if prop.Type != "" {
		p.Type = prop.Type
	}
	if prop.FormGroup != "" {
		p.FormGroup = prop.FormGroup
	}
	if prop.FormTab != "" {
		p.FormTab = prop.FormTab
	}
	if prop.FormOrder != 0 {
		p.FormOrder = prop.FormOrder
	}
	if prop.Access != nil {
		p.Access = prop.Access
	}
	if prop.Display != "" {
		p.Display = prop.Display
	}
	// TODO придумать как поступать с булевыми полями. Если оно отсутствует в JSON, то всегда будет false
	p.ReadOnly = prop.ReadOnly
	p.Required = prop.Required

	if prop.Default != nil {
		p.Default = prop.Default
	}
	if prop.MinLength != 0 {
		p.MinLength = prop.MinLength
	}
	if prop.MaxLength != 0 {
		p.MaxLength = prop.MaxLength
	}
	if prop.Min != nil {
		p.Min = prop.Min
	}
	if prop.Max != nil {
		p.Max = prop.Max
	}
	if prop.Hidden != nil {
		p.Hidden = prop.Hidden
	}
	if prop.Pattern != nil {
		p.Pattern = prop.Pattern
	}
	if prop.Mask != nil {
		p.Mask = prop.Mask
	}
	if prop.Load != "" {
		p.Load = prop.Load
	}
	if prop.Store != "" {
		p.Store = prop.Store
	}
	if prop.PopulateIn != nil {
		p.PopulateIn = prop.PopulateIn
	}
	if prop.Label != "" {
		p.Label = prop.Label
	}
	if prop.Placeholder != "" {
		p.Placeholder = prop.Placeholder
	}
	if prop.Disabled != "" {
		p.Disabled = prop.Disabled
	}
	if len(prop.SearchBy) != 0 {
		p.SearchBy = prop.SearchBy
	}
	if prop.OppositeProp != "" {
		p.OppositeProp = prop.OppositeProp
	}

	s.Props[name] = p
}

func (s *Store) mergeAccess(defaultStore *Store) {
	if len(s.Access) == 0 && len(defaultStore.Access) > 0 {
		for i := range defaultStore.Access {
			s.Access = append(s.Access, defaultStore.Access[i])
		}
	}
}

func (s *Store) mergeFilters(defaultStore *Store) {
	if len(defaultStore.Filters) == 0 {
		return
	}
	if len(s.Filters) == 0 {
		s.Filters = map[string]Filter{}
	}
	for k, v := range defaultStore.Filters {
		f, ok := s.Filters[k]
		if !ok {
			s.Filters[k] = v
			continue
		}
		if f.Label == "" {
			f.Label = v.Label
		}
		if f.Display == "" {
			f.Display = v.Display
		}
		if f.Placeholder == "" {
			f.Placeholder = v.Placeholder
		}
		if len(f.SearchBy) == 0 {
			f.SearchBy = v.SearchBy
		}
		if f.Store == "" {
			f.Store = v.Store
		}
		if f.FilterBy == "" {
			f.FilterBy = v.FilterBy
		}
		if len(f.Options) == 0 {
			f.Options = v.Options
		}
		if f.Mask == "" {
			f.Mask = v.Mask
		}
		if !f.Multi {
			f.Multi = v.Multi
		}
		s.Filters[k] = f
	}
}

func (s *Store) mergeWith(src Store) {
	// Access
	// Actions
	if len(src.BaseStore) > 0 {
		s.BaseStore = src.BaseStore
	}
	// Config
	if src.DataSource != nil {
		s.DataSource = src.DataSource
	}

	// TODO: fill all props
	if len(src.Label) > 0 {
		s.Label = src.Label
	}
}
