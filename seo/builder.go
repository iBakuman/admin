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
	GlobalDB     *gorm.DB
	DBContextKey contextKey = "DB"
)

type (
	contextKey           string
	contextVariablesFunc func(interface{}, *Setting, *http.Request) string
)

func NewBuilder() *Builder {
	b := &Builder{
		dbContextKey:  DBContextKey,
		registeredSEO: make(map[string]*SEO),
		dummyNode:     &SEO{},
	}

	return b
}

// Builder will hold registered SEO configures and global setting definition and other configures
// @snippet_begin(SeoBuilderDefinition)
type Builder struct {
	// key == val.Name
	registeredSEO map[string]*SEO

	dummyNode    *SEO
	dbContextKey interface{}                                                        // get db from context
	afterSave    func(ctx context.Context, settingName string, locale string) error // hook called after saving
}

// @snippet_end

// SetDBContextKey sets the key to get db instance from context
func (b *Builder) SetDBContextKey(key interface{}) *Builder {
	b.dbContextKey = key
	return b
}

// RegisterMultipleSEO registers multiple SEOs.
// It calls RegisterSEO to accomplish its functionality.
func (b *Builder) RegisterMultipleSEO(objs ...interface{}) []*SEO {
	SEOs := make([]*SEO, 0, len(objs))
	for _, obj := range objs {
		SEOs = append(SEOs, b.RegisterSEO(obj))
	}
	return SEOs
}

// RegisterSEO registers a SEO through name or model.
// If an SEO already exists, it will panic.
// The obj parameter can be of type string or a struct type that nested Setting.
// The default parent of the registered SEO is dummyNode. If you need to set
// its parent, Please call the SetParent method of SEO after invoking RegisterSEO method.
// For Example: b.RegisterSEO(&Region{}).SetParent(parentSEO)
func (b *Builder) RegisterSEO(obj interface{}) *SEO {
	if obj == nil {
		panic("cannot register nil SEO, SEO must be of type string or struct type that nested Setting")
	}
	seoName := GetSEOName(obj)
	if seoName == "" {
		panic("the seo name must not be empty")
	}
	b.GetSEO(seoName)
	if _, isExist := b.registeredSEO[seoName]; isExist {
		panic(fmt.Sprintf("The %v SEO already exists!", seoName))
	}
	// default parent is dummyNode
	seo := &SEO{name: seoName}
	seo.SetParent(b.dummyNode)
	if _, ok := obj.(string); !ok { // for model SEO
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
			panic("obj must be of type string or struct type that nested Setting")
		}
	}
	b.registeredSEO[seoName] = seo
	return seo
}

// RemoveSEO removes the specified SEO,
// if the SEO has children, the parent of the children will
// be the parent of the SEO
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
// the SEO name is obtained from the type name that is retrieved through reflection.
// If no SEO with the specified name is found, it returns nil.
func (b *Builder) GetSEO(obj interface{}) *SEO {
	name := GetSEOName(obj)
	return b.registeredSEO[name]
}

// GetSEOPriority gets the priority of the specified SEO,
// with higher number indicating higher priority.
// The priority of Global SEO is 1 (the lowest priority)
func (b *Builder) GetSEOPriority(name string) int {
	node := b.GetSEO(name)
	depth := -1
	for node != nil {
		node = node.parent
		depth++
	}
	return depth
}

func (b *Builder) SortSEOs(SEOs []*QorSEOSetting) {
	m := make(map[string]int)
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
	dfs(b.dummyNode)
	sort.Slice(SEOs, func(i, j int) bool {
		return m[SEOs[i].Name] < m[SEOs[j].Name]
	})
}

// AfterSave sets the hook called after saving
func (b *Builder) AfterSave(v func(ctx context.Context, settingName string, locale string) error) *Builder {
	b.afterSave = v
	return b
}

func (b *Builder) Render(obj interface{}, req *http.Request) h.HTMLComponent {
	seo := b.GetSEO(obj)
	if seo == nil {
		return h.RawHTML("")
	}

	var locale string
	if v, ok := obj.(l10n.L10nInterface); ok {
		locale = v.GetLocale()
	}

	db := b.getDBFromContext(req.Context())
	finalSeoSetting := seo.getFinalQorSEOSetting(locale, db)

	// get setting
	var setting Setting
	{
		setting = finalSeoSetting.Setting
		if _, ok := obj.(string); !ok {
			if value := reflect.Indirect(reflect.ValueOf(obj)); value.IsValid() && value.Kind() == reflect.Struct {
				for i := 0; i < value.NumField(); i++ {
					if value.Field(i).Type() == reflect.TypeOf(Setting{}) {
						if tSetting := value.Field(i).Interface().(Setting); tSetting.EnabledCustomize {
							// if the obj embeds Setting, then overrides `finalSeoSetting.Setting` with `tSetting`
							setting = tSetting
						}
						break
					}
				}
			}
		}
	}

	// replace placeholders
	{
		variables := finalSeoSetting.Variables
		finalContextVars := seo.getFinalContextVars()
		// execute function for context var
		for varName, varFunc := range finalContextVars {
			variables[varName] = varFunc(obj, &setting, req)
		}
		setting = replaceVariables(setting, variables)
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
	}

	ogProps := map[string]string{}
	{
		finalPropFuncForOG := seo.getFinalPropFuncForOG()
		for propName, propFunc := range finalPropFuncForOG {
			ogProps[propName] = propFunc(obj, &setting, req)
		}
	}

	return setting.HTMLComponent(ogProps)
}

func (b *Builder) BatchRender(models []interface{}) ([]h.HTMLComponent, error) {
	return nil, nil
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

// GetSEOName return the SEO name.
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
