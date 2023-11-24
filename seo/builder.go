package seo

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"reflect"
	"regexp"
	"sort"
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

func NewBuilder() *Builder {
	b := &Builder{
		dbContextKey:  DBContextKey,
		globalName:    GlobalSEOName,
		inherited:     true,
		registeredSEO: make(map[string]*SEO),
	}

	b.RegisterSEO(GlobalSEOName).RegisterSettingVariables(struct{ SiteName string }{}).
		RegisterContextVariables(
			"og:url", func(_ interface{}, _ *Setting, req *http.Request) string {
				return req.URL.String()
			},
		)

	return b
}

// Builder will hold registered seo configures and global setting definition and other configures
// @snippet_begin(SeoBuilderDefinition)
type Builder struct {
	// key == val.Name
	registeredSEO map[string]*SEO
	globalName    string                                                             // default name is GlobalSEOName
	inherited     bool                                                               // default is true. the order is model seo setting, system seo setting, global seo setting
	dbContextKey  interface{}                                                        // get db from context
	afterSave     func(ctx context.Context, settingName string, locale string) error // hook called after saving
	l10n          *l10n.Builder

	prioritiesCatches   map[string]int
	listingOrderCatches map[string]int
}

// @snippet_end

func (b *Builder) L10nBuilder(builder *l10n.Builder) *Builder {
	b.l10n = builder
	return b
}
func (b *Builder) SetGlobalName(name string) *Builder {
	globalSEO := b.GetGlobalSEO()
	globalSEO.SetName(name)
	b.globalName = name
	delete(b.registeredSEO, GlobalSEOName)
	return b
}

func (b *Builder) GetGlobalSEO() *SEO {
	return b.registeredSEO[b.globalName]
}

func (b *Builder) SetInherited(inherited bool) *Builder {
	b.inherited = inherited
	return b
}

// SetDBContextKey sets the key to get db instance from context
func (b *Builder) SetDBContextKey(key interface{}) *Builder {
	b.dbContextKey = key
	return b
}

// RegisterMultipleSEO registers multiple SEOs.
// It calls RegisterSEO to accomplish the functionality.
func (b *Builder) RegisterMultipleSEO(objs ...interface{}) []*SEO {
	SEOs := make([]*SEO, 0, len(objs))
	for _, obj := range objs {
		SEOs = append(SEOs, b.RegisterSEO(obj))
	}
	return SEOs
}

// RegisterSEO registers a seo through name or model.
// If an SEO already exists, it will panic.
// The obj parameter can be of type string or a struct type that nested Setting.
// The default parent of the registered SEO is GlobalSEO. If you need to set
// its parent, Please call the SetParent method of SEO  after invoking RegisterSEO method.
// For Example: b.RegisterSEO(&Region{}).SetParent(parent)
func (b *Builder) RegisterSEO(obj interface{}) (seo *SEO) {
	if obj == nil {
		panic("cannot register nil seo, seo must be of type string or struct type that nested Setting")
	}
	seoName := GetSEOName(obj)
	b.GetSEO(seoName)
	if _, isExist := b.registeredSEO[seoName]; isExist {
		panic(fmt.Sprintf("The %v seo already exists!", seoName))
	}
	// default parent is Global SEO
	globalSEO := b.GetGlobalSEO()
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
	b.registeredSEO[seoName] = seo
	return
}

// RemoveSEO removes the specified seo,
// if the seo has children, the parent of the children will
// be the parent of the seo
func (b *Builder) RemoveSEO(obj interface{}) *Builder {
	seoToBeRemoved := b.GetSEO(obj)
	if seoToBeRemoved == nil {
		return b
	}
	seoToBeRemoved.RemoveSelf()
	delete(b.registeredSEO, seoToBeRemoved.name)
	return b
}

// GetSEO retrieves the specified SEO, It accepts two types of parameters.
// One is a string, where the literal value of the parameter is the name of the SEO.
// The other is an instance of a struct embedded with the Setting type, in which case
// the seo name is obtained from the type name that is retrieved through reflection.
// If no SEO with the specified name is found, it returns nil.
func (b *Builder) GetSEO(obj interface{}) *SEO {
	name := GetSEOName(obj)
	return b.registeredSEO[name]
}

// GetSEOPriority gets the priority of the specified seo,
// with higher number indicating higher priority.
// The priority of Global SEO is 1 (the lowest priority)
func (b *Builder) GetSEOPriority(name string) int {
	depth := 0
	node := b.registeredSEO[name]
	for node != nil {
		node = node.parent
		depth++
	}
	return depth
}

func (b *Builder) SortSEOs(SEOs []*QorSEOSetting) {
	m := make(map[string]int)
	globalSEO := b.GetGlobalSEO()
	order := 0
	var dfs func(root *SEO)
	dfs = func(seo *SEO) {
		if seo == nil {
			return
		}
		m[seo.name] = order
		order++
		for _, child := range seo.children {
			dfs(child)
		}
	}
	dfs(globalSEO)
	sort.Slice(SEOs, func(i, j int) bool {
		return m[SEOs[i].Name] < m[SEOs[j].Name]
	})
}

// AfterSave sets the hook called after saving
func (b *Builder) AfterSave(v func(ctx context.Context, settingName string, locale string) error) *Builder {
	b.afterSave = v
	return b
}

// RenderGlobal renders global SEO
func (b *Builder) RenderGlobal(req *http.Request) h.HTMLComponent {
	return b.Render(b.globalName, req)
}

// Render render seo tags
func (b *Builder) Render(obj interface{}, req *http.Request) h.HTMLComponent {
	var (
		db               = b.getDBFromContext(req.Context())
		sortedSEOs       []*SEO
		sortedSeoNames   []string
		sortedDBSettings []*QorSEOSetting
		sortedSettings   []Setting
		setting          Setting
		locale           string
	)

	// sort all SEOs
	globalSeo := b.GetGlobalSEO()
	if globalSeo == nil {
		return h.RawHTML("")
	}

	sortedSEOs = append(sortedSEOs, globalSeo)

	if name, ok := obj.(string); !ok || name != b.globalName {
		if seo := b.GetSEO(obj); seo != nil {
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
	var settingModelSlice []*QorSEOSetting
	if db.Find(&settingModelSlice, "name in (?) AND locale_code = ?", sortedSeoNames, locale).Error != nil {
		return h.RawHTML("")
	}

	for _, name := range sortedSeoNames {
		for _, tSetting := range settingModelSlice {
			if tSetting.Name == name {
				sortedDBSettings = append(sortedDBSettings, tSetting)
			}
		}
	}

	// model {
	// setting Setting
	// }
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
		sortedSettings = append(sortedSettings, s.Setting)
	}

	// get the final setting from sortedSettings
	for i, s := range sortedSettings {
		if !b.inherited && i >= 1 {
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
		for key, val := range s.Variables {
			variables[key] = val
		}
	}

	for i, seo := range sortedSEOs {
		for key, f := range seo.contextVariables {
			value := f(obj, &setting, req)
			if strings.Contains(key, ":") && b.inherited {
				tags[key] = value
			} else if strings.Contains(key, ":") && !b.inherited && i == 0 {
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
func (b *Builder) getDBFromContext(ctx context.Context) *gorm.DB {
	if ctxDB := ctx.Value(b.dbContextKey); ctxDB != nil {
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
