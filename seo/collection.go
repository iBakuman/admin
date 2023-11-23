package seo

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"reflect"
	"regexp"
	"strings"

	"github.com/qor5/admin/l10n"

	h "github.com/theplant/htmlgo"
	"gorm.io/gorm"
)

var (
	GlobalSEOName = "Global SEO"
	GlobalDB      *gorm.DB
	DBContextKey  contextKey = "DB"
)

type (
	contextKey           string
	contextVariablesFunc func(interface{}, *Setting, *http.Request) string
)

// NewCollection creates a new SeoCollection instance
func NewCollection() *Collection {
	collection := &Collection{
		settingModel:  &QorSEOSetting{},
		dbContextKey:  DBContextKey,
		globalName:    GlobalSEOName,
		inherited:     true,
		registeredSEO: make(map[string]*SEO),
	}

	collection.RegisterSEO(GlobalSEOName).RegisterSettingVaribles(struct{ SiteName string }{}).
		RegisterContextVariables(
			"og:url", func(_ interface{}, _ *Setting, req *http.Request) string {
				return req.URL.String()
			},
		)

	return collection
}

// Collection will hold registered seo configures and global setting definition and other configures
// @snippet_begin(SeoCollectionDefinition)
type Collection struct {
	// key == val.Name
	registeredSEO map[string]*SEO
	globalName    string                                                             // default name is GlobalSEOName
	inherited     bool                                                               // default is true. the order is model seo setting, system seo setting, global seo setting
	dbContextKey  interface{}                                                        // get db from context
	settingModel  QorSEOSettingInterface                                             // db model
	afterSave     func(ctx context.Context, settingName string, locale string) error // hook called after saving

	prioritiesCatches   map[string]int
	listingOrderCatches map[string]int
}

// @snippet_end

func (collection *Collection) SetGlobalName(name string) *Collection {
	globalSEO := collection.GetGlobalSEO()
	globalSEO.SetName(name)
	collection.globalName = name
	delete(collection.registeredSEO, GlobalSEOName)
	return collection
}

func (collection *Collection) GetGlobalSEO() *SEO {
	return collection.registeredSEO[collection.globalName]
}

func (collection *Collection) NewSettingModelInstance() interface{} {
	return reflect.New(reflect.Indirect(reflect.ValueOf(collection.settingModel)).Type()).Interface()
}

func (collection *Collection) NewSettingModelSlice() interface{} {
	sliceType := reflect.SliceOf(reflect.PtrTo(reflect.Indirect(reflect.ValueOf(collection.settingModel)).Type()))
	slice := reflect.New(sliceType)
	slice.Elem().Set(reflect.MakeSlice(sliceType, 0, 0))
	return slice.Interface()
}

func (collection *Collection) SetInherited(b bool) *Collection {
	collection.inherited = b
	return collection
}

func (collection *Collection) SetSettingModel(s QorSEOSettingInterface) *Collection {
	collection.settingModel = s
	return collection
}

// SetDBContextKey sets the key to get db instance from context
func (collection *Collection) SetDBContextKey(key interface{}) *Collection {
	collection.dbContextKey = key
	return collection
}

func (collection *Collection) RegisterSEOByNames(names ...string) *Collection {
	for _, name := range names {
		collection.RegisterSEO(name)
	}
	return collection
}

// RegisterMultipleSEO registers multiple SEOs.
// It calls RegisterSEO to accomplish the functionality.
func (collection *Collection) RegisterMultipleSEO(objs ...interface{}) []*SEO {
	seos := make([]*SEO, 0, len(objs))
	for _, obj := range objs {
		seos = append(seos, collection.RegisterSEO(obj))
	}
	return seos
}

// RegisterSEO registers a seo through name or model.
// If an SEO already exists, it will panic.
// The obj parameter can be of type string or a struct type that nested Setting.
// The default parent of the registered SEO is GlobalSEO. If you need to set
// its parent, Please call the SetParent method of SEO  after invoking RegisterSEO method.
// For Example: collection.RegisterSEO(&Region{}).SetParent(parent)
func (collection *Collection) RegisterSEO(obj interface{}) (seo *SEO) {
	if obj == nil {
		panic("cannot register nil seo, seo must be of type string or struct type that nested Setting")
	}
	seoName := GetSEOName(obj)
	collection.GetSEOByName(seoName)
	if _, isExist := collection.registeredSEO[seoName]; isExist {
		panic(fmt.Sprintf("The %v seo already exists!", seoName))
	}
	// default parent is Global SEO
	globalSEO := collection.GetGlobalSEO()
	seo = &SEO{name: seoName}
	seo.SetParent(globalSEO)
	if _, ok := obj.(string); !ok { // for model seo
		seo.modelTyp = reflect.Indirect(reflect.ValueOf(obj)).Type()
		isSettingNested := false
		if value := reflect.Indirect(reflect.ValueOf(obj)); value.IsValid() && value.Kind() == reflect.Struct {
			for i := 0; i < value.NumField(); i++ {
				if value.Field(i).Type() == reflect.TypeOf(Setting{}) {
					isSettingNested = true
					seo.modelTyp = value.Type()
					break
				}
			}
		}
		if !isSettingNested {
			panic("seo must be of type string or struct type that nested Setting")
		}
	}
	collection.registeredSEO[seoName] = seo
	return
}

// RemoveSEO removes the specified seo,
// if the seo has children, the parent of the children will
// be the parent of the seo
func (collection *Collection) RemoveSEO(obj interface{}) *Collection {
	seoToBeRemoved := collection.GetSEO(obj)
	if seoToBeRemoved == nil {
		return collection
	}
	seoToBeRemoved.RemoveSelf()
	delete(collection.registeredSEO, seoToBeRemoved.name)
	return collection
}

// GetSEO gets the specified SEO by name or model.
// It calls methods GetSEOByName and GetSEOByModel to realize its functionality.
func (collection *Collection) GetSEO(obj interface{}) *SEO {
	name := GetSEOName(obj)
	return collection.GetSEOByName(name)
}

// GetSEOByName gets the specified SEO by the name.
func (collection *Collection) GetSEOByName(name string) *SEO {
	return collection.registeredSEO[name]
}

// GetSEOByModel gets a seo by model,
func (collection *Collection) GetSEOByModel(model interface{}) *SEO {
	name := GetSEOName(model)
	return collection.GetSEOByName(name)
}

// GetSEOPriorities gets the priorities of all SEOs, with higher number
// indicating higher priority. The priority of Global SEO is 1 (the lowest priority)
func (collection *Collection) GetSEOPriorities() map[string]int {
	res := make(map[string]int)
	var dfs func(seo *SEO) int
	dfs = func(seo *SEO) int {
		if seo == nil {
			return 0
		}
		if _, ok := res[seo.name]; ok {
			return res[seo.name]
		}
		res[seo.name] = dfs(seo.parent) + 1
		return res[seo.name]
	}
	for _, seo := range collection.registeredSEO {
		dfs(seo)
	}
	return res
}

func (collection *Collection) GetListingOrders() map[string]int {
	listingOrders := make(map[string]int)
	globalSEO := collection.GetGlobalSEO()
	order := 0
	var dfs func(root *SEO)
	dfs = func(seo *SEO) {
		if seo == nil {
			return
		}
		listingOrders[seo.name] = order
		order++
		for _, child := range seo.children {
			dfs(child)
		}
	}
	dfs(globalSEO)
	return listingOrders
}

// AfterSave sets the hook called after saving
func (collection *Collection) AfterSave(v func(ctx context.Context, settingName string, locale string) error) *Collection {
	collection.afterSave = v
	return collection
}

// RenderGlobal renders global SEO
func (collection *Collection) RenderGlobal(req *http.Request) h.HTMLComponent {
	return collection.Render(collection.globalName, req)
}

// Render render seo tags
func (collection *Collection) Render(obj interface{}, req *http.Request) h.HTMLComponent {
	var (
		db               = collection.getDBFromContext(req.Context())
		sortedSEOs       []*SEO
		sortedSeoNames   []string
		sortedDBSettings []QorSEOSettingInterface
		sortedSettings   []Setting
		setting          Setting
		locale           string
	)

	// sort all SEOs
	globalSeo := collection.GetGlobalSEO()
	if globalSeo == nil {
		return h.RawHTML("")
	}

	sortedSEOs = append(sortedSEOs, globalSeo)

	if name, ok := obj.(string); !ok || name != collection.globalName {
		if seo := collection.GetSEO(obj); seo != nil {
			sortedSeoNames = append(sortedSeoNames, seo.name)
			// global seo -- model seo
			sortedSEOs = append(sortedSEOs, seo)
		}
	}
	// system seo name -- global seo name
	sortedSeoNames = append(sortedSeoNames, globalSeo.name)

	if v, ok := obj.(l10n.L10nInterface); ok {
		locale = v.GetLocale()
	}

	// sort all QorSEOSettingInterface
	var settingModelSlice = collection.NewSettingModelSlice()
	if db.Find(settingModelSlice, "name in (?) AND locale_code = ?", sortedSeoNames, locale).Error != nil {
		return h.RawHTML("")
	}

	reflectValue := reflect.Indirect(reflect.ValueOf(settingModelSlice))

	for _, name := range sortedSeoNames {
		for i := 0; i < reflectValue.Len(); i++ {
			if modelSetting, ok := reflectValue.Index(i).Interface().(QorSEOSettingInterface); ok && modelSetting.GetName() == name {
				// model seo name -- global seo name
				sortedDBSettings = append(sortedDBSettings, modelSetting)
			}
		}
	}

	// sort all settings
	if _, ok := obj.(string); !ok {
		if value := reflect.Indirect(reflect.ValueOf(obj)); value.IsValid() && value.Kind() == reflect.Struct {
			for i := 0; i < value.NumField(); i++ {
				if value.Field(i).Type() == reflect.TypeOf(Setting{}) {
					if setting := value.Field(i).Interface().(Setting); setting.EnabledCustomize {
						sortedSettings = append(sortedSettings, setting)
					}
					break
				}
			}
		}
	}

	for _, s := range sortedDBSettings {
		// instance seo -- model seo -- global seo
		sortedSettings = append(sortedSettings, s.GetSEOSetting())
	}

	// get the final setting from sortedSettings
	for i, s := range sortedSettings {
		if !collection.inherited && i >= 1 {
			break
		}

		if s.Title != "" && setting.Title == "" {
			setting.Title = s.Title
		}
		if s.Description != "" && setting.Description == "" {
			setting.Description = s.Description
		}
		if s.Keywords != "" && setting.Keywords == "" {
			setting.Keywords = s.Keywords
		}
		if s.OpenGraphTitle != "" && setting.OpenGraphTitle == "" {
			setting.OpenGraphTitle = s.OpenGraphTitle
		}
		if s.OpenGraphDescription != "" && setting.OpenGraphDescription == "" {
			setting.OpenGraphDescription = s.OpenGraphDescription
		}
		if s.OpenGraphURL != "" && setting.OpenGraphURL == "" {
			setting.OpenGraphURL = s.OpenGraphURL
		}
		if s.OpenGraphType != "" && setting.OpenGraphType == "" {
			setting.OpenGraphType = s.OpenGraphType
		}
		if s.OpenGraphImageURL != "" && setting.OpenGraphImageURL == "" {
			setting.OpenGraphImageURL = s.OpenGraphImageURL
		}
		if s.OpenGraphImageFromMediaLibrary.URL("og") != "" && setting.OpenGraphImageURL == "" {
			setting.OpenGraphImageURL = s.OpenGraphImageFromMediaLibrary.URL("og")
		}
		if len(s.OpenGraphMetadata) > 0 && len(setting.OpenGraphMetadata) == 0 {
			setting.OpenGraphMetadata = s.OpenGraphMetadata
		}
	}

	if setting.OpenGraphURL != "" && !isAbsoluteURL(setting.OpenGraphURL) {
		var u url.URL
		u.Host = req.Host
		if req.URL.Scheme != "" {
			u.Scheme = req.URL.Scheme
		} else {
			u.Scheme = "http"
		}
		setting.OpenGraphURL = path.Join(u.String(), setting.OpenGraphURL)
	}

	// fetch all variables and tags from context
	var (
		variables = map[string]string{}
		tags      = map[string]string{}
	)

	for _, s := range sortedDBSettings {
		for key, val := range s.GetVariables() {
			variables[key] = val
		}
	}

	for i, seo := range sortedSEOs {
		for key, f := range seo.contextVariables {
			value := f(obj, &setting, req)
			if strings.Contains(key, ":") && collection.inherited {
				tags[key] = value
			} else if strings.Contains(key, ":") && !collection.inherited && i == 0 {
				tags[key] = value
			} else {
				variables[key] = f(obj, &setting, req)
			}
		}
	}
	setting = replaceVariables(setting, variables)
	return setting.HTMLComponent(tags)
}

// getDBFromContext get the db from the ctx
func (collection *Collection) getDBFromContext(ctx context.Context) *gorm.DB {
	if ctxDB := ctx.Value(collection.dbContextKey); ctxDB != nil {
		return ctxDB.(*gorm.DB)
	}
	return GlobalDB
}

var regex = regexp.MustCompile("{{([a-zA-Z0-9]*)}}")

func replaceVariables(setting Setting, values map[string]string) Setting {
	replace := func(str string) string {
		matches := regex.FindAllStringSubmatch(str, -1)
		for _, match := range matches {
			str = strings.Replace(str, match[0], values[match[1]], 1)
		}
		return str
	}

	setting.Title = replace(setting.Title)
	setting.Description = replace(setting.Description)
	setting.Keywords = replace(setting.Keywords)
	setting.OpenGraphTitle = replace(setting.OpenGraphTitle)
	setting.OpenGraphDescription = replace(setting.OpenGraphDescription)
	setting.OpenGraphURL = replace(setting.OpenGraphURL)
	setting.OpenGraphType = replace(setting.OpenGraphType)
	setting.OpenGraphImageURL = replace(setting.OpenGraphImageURL)
	var metadata []OpenGraphMetadata
	for _, m := range setting.OpenGraphMetadata {
		metadata = append(metadata, OpenGraphMetadata{
			Property: m.Property,
			Content:  replace(m.Content),
		})
	}
	setting.OpenGraphMetadata = metadata
	return setting
}

func isAbsoluteURL(str string) bool {
	if u, err := url.Parse(str); err == nil && u.IsAbs() {
		return true
	}
	return false
}

func ContextWithDB(ctx context.Context, db *gorm.DB) context.Context {
	return context.WithValue(ctx, DBContextKey, db)
}

// GetSEOName return the seo name.
// if obj is of type string, its literal value is returned,
// if obj is of any other type, the name of its type is returned.
func GetSEOName(obj interface{}) string {
	switch res := obj.(type) {
	case string:
		return res
	default:
		return reflect.Indirect(reflect.ValueOf(obj)).Type().Name()
	}
}
