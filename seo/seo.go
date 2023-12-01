package seo

import (
	"fmt"
	"github.com/qor5/admin/l10n"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"reflect"
	"strings"
)

type ContextVarType int

const (
	OpenGraph ContextVarType = iota
	Plain
)

// SEO represents a SEO object for a page
// @snippet_begin(SeoDefinition)
type SEO struct {
	// The parent field represents the upper-level SEO for this SEO.
	// When certain fields are not presents in the current SEO,
	// the values for these fields will be retrieved from the parent SEO.
	// NOTE: The parent of Global SEO is nil.
	parent *SEO

	// children records SEOs with higher priority than the current SEO.
	// when certain fields are missing in these SEOs, they can refer to
	// the current SEO to obtain these fields.
	children []*SEO

	// The name field is a unique identifier and cannot be duplicated.
	// Registering Multiple SEOs with the same name in Builder will cause
	// the program to panic.
	name string

	modelTyp reflect.Type

	// [The Optional Metadata for Open Graph Protocol](https://ogp.me/#optional)
	propFuncForOG map[string]contextVariablesFunc

	// Dynamically retrieve the content that replace the placeholders with its value
	contextVars map[string]contextVariablesFunc

	// Replace the placeholders within {{}} with the values from the variable field in the database.
	// For example, if the variable field in the database contains a:"b", then {{a}} will be replaced with b.
	settingVars map[string]struct{}

	finalContextVarsCache   map[string]contextVariablesFunc
	finalPropFuncForOGCache map[string]contextVariablesFunc
	finalAvailableVarsCache map[string]struct{}
}

// @snippet_end

// AppendChildren sets the parent of each child in the children to the current SEO.
// Usage:
// builder.RegisterSEO(parent).AppendChildren(
//
//	builder.RegisterSEO(child1),
//	builder.RegisterSEO(child2),
//	builder.RegisterSEO(child3)
//
// )
// Or:
// builder.RegisterSEO(parent).AppendChildren(
//
//	builder.RegisterMultipleSEO(child1, child2, child3)
//
// )
func (seo *SEO) AppendChildren(children ...*SEO) *SEO {
	if seo == nil {
		return seo
	}

	for _, child := range children {
		if child == nil {
			panic("cannot add nil as child")
		}
		child.SetParent(seo)
	}
	return seo
}

// SetParent sets the parent of the current SEO node to newParent
// if newParent is already the parent of current SEO node, it returns itself directly.
// if newParent is the descendant node of the current SEO node, it panics.
// if newParent is
func (seo *SEO) SetParent(newParent *SEO) *SEO {
	// 1. SEO is nil
	// 2. newParent is already the parent node of SEO
	if seo == nil || seo.parent == newParent {
		return seo
	}

	// Check if the new parent is a descendant node of the current SEO node.
	// if it is, panic directly to prevent the creation of a cycle.
	{
		node := newParent
		for node != nil {
			if node == seo {
				panic("cannot assign a descendant node as the parent node of the current node")
			}
			node = node.parent
		}
	}

	// remove current SEO node from oldParent
	oldParent := seo.parent
	if oldParent != nil {
		for i, childOfOldParent := range oldParent.children {
			if childOfOldParent == seo {
				oldParent.children = append(oldParent.children[:i], oldParent.children[i+1:]...)
				break
			}
		}
	}

	// set the parent of the current SEO node to newParent
	seo.parent = newParent
	if newParent != nil {
		newParent.children = append(newParent.children, seo)
	}

	// check conflict
	for varName := range seo.settingVars {
		seo.checkConflict(varName, false)
	}
	for varName := range seo.contextVars {
		seo.checkConflict(varName, true)
	}
	return seo
}

// RemoveSelf removes itself from the SEO tree.
// the parent of its every child will be changed to the parent of it
func (seo *SEO) RemoveSelf() *SEO {
	if seo == nil {
		return seo
	}
	if seo.parent != nil {
		for i, childOfParent := range seo.parent.children {
			if childOfParent == seo {
				seo.parent.children = append(seo.parent.children[:i], seo.parent.children[i+1:]...)
				break
			}
		}
	}
	if len(seo.children) > 0 {
		for _, child := range seo.children {
			// Note: Do not use "child.parent = SEO.parent"
			child.SetParent(seo.parent)
		}
	}
	return seo
}

type ContextVar struct {
	Name string
	Func contextVariablesFunc
}

func (seo *SEO) RegisterContextVariables(contextVars ...*ContextVar) *SEO {
	if seo == nil {
		return nil
	}
	if seo.contextVars == nil {
		seo.contextVars = make(map[string]contextVariablesFunc)
	}
	for _, contextVar := range contextVars {
		varName, varFunc := strings.TrimSpace(contextVar.Name), contextVar.Func
		if varName == "" {
			panic("The name of context var must not be empty")
		}
		if _, isExist := seo.contextVars[varName]; isExist {
			panic(fmt.Sprintf("The context variable %v has already been registered", varName))
		}
		if varFunc == nil {
			panic("The function of context var must not be nil")
		}
		seo.checkConflict(varName, true)
		seo.contextVars[varName] = varFunc
	}
	return seo
}

func (seo *SEO) RegisterSettingVariables(names ...string) *SEO {
	if seo == nil {
		return seo
	}
	if seo.settingVars == nil {
		seo.settingVars = make(map[string]struct{})
	}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			panic("The name of setting var must not be empty")
		}
		seo.checkConflict(name, false)
		seo.settingVars[name] = struct{}{}
	}
	return seo
}

type PropFunc struct {
	Name string
	Func contextVariablesFunc
}

func (seo *SEO) RegisterPropFuncForOG(propFuncs ...*PropFunc) *SEO {
	if seo == nil {
		return nil
	}
	if seo.propFuncForOG == nil {
		seo.propFuncForOG = make(map[string]contextVariablesFunc)
	}
	for _, propAndFunc := range propFuncs {
		propName := propAndFunc.Name
		propFunc := propAndFunc.Func

		prop := strings.TrimSpace(propName)
		if prop == "" || propFunc == nil {
			panic("Both property name and function are required")
		}
		if !strings.Contains(prop, ":") {
			panic(fmt.Sprintf("%v is not a valid OpenGraph property name", prop))
		}
		if _, isExist := seo.propFuncForOG[prop]; isExist {
			panic(fmt.Sprintf("property %v has already been registered", prop))
		}
		seo.propFuncForOG[prop] = propFunc
	}
	return seo
}

func (seo *SEO) getFinalPropFuncForOG() map[string]contextVariablesFunc {
	if seo == nil {
		return nil
	}
	if seo.finalPropFuncForOGCache != nil {
		return seo.finalPropFuncForOGCache
	} else {
		seo.finalPropFuncForOGCache = make(map[string]contextVariablesFunc)
		for propName, propFunc := range seo.propFuncForOG {
			seo.finalPropFuncForOGCache[propName] = propFunc
		}
		cacheOfParent := seo.parent.getFinalPropFuncForOG()
		for propName, propFunc := range cacheOfParent {
			if _, isExist := seo.finalPropFuncForOGCache[propName]; !isExist {
				seo.finalPropFuncForOGCache[propName] = propFunc
			}
		}
		return seo.finalPropFuncForOGCache
	}
}

func (seo *SEO) getAvailableVars() map[string]struct{} {
	if seo == nil {
		return nil
	}
	if seo.finalAvailableVarsCache != nil {
		return seo.finalAvailableVarsCache
	} else {
		seo.finalAvailableVarsCache = make(map[string]struct{})
		for varName := range seo.settingVars {
			seo.finalAvailableVarsCache[varName] = struct{}{}
		}
		for varName := range seo.contextVars {
			seo.finalAvailableVarsCache[varName] = struct{}{}
		}
		for varName, val := range seo.parent.getAvailableVars() {
			seo.finalAvailableVarsCache[varName] = val
		}
		return seo.finalAvailableVarsCache
	}
}

func (seo *SEO) migrate(locales []string) {
	if seo == nil {
		return
	}

	// Checking `seo.name != nil` is to avoid create records with empty name
	// when the current node is the dummy node
	if seo.name != "" {
		settings := make([]QorSEOSetting, 0, len(locales))
		variables := make(map[string]string)
		for varName := range seo.settingVars {
			variables[varName] = ""
		}
		if len(locales) == 0 {
			settings = append(settings, QorSEOSetting{
				Name:      seo.name,
				Variables: variables,
				Locale:    l10n.Locale{LocaleCode: "empty"},
			})
		} else {
			for _, locale := range locales {
				settings = append(settings, QorSEOSetting{
					Name:      seo.name,
					Locale:    l10n.Locale{LocaleCode: locale},
					Variables: variables,
				})
			}
		}
		// The aim to use `Clauses(clause.OnConflict{DoNothing: true})` is it will not affect the existing data
		// or cause the create function to fail When the data to be inserted already exists in the database,
		if err := dbForTest.Clauses(clause.OnConflict{DoNothing: true}).Create(&settings).Error; err != nil {
			panic(err)
		}
	}

	for _, child := range seo.children {
		child.migrate(locales)
	}
	return
}

func (seo *SEO) getFinalQorSEOSetting(locale string, db *gorm.DB) *QorSEOSetting {
	if seo == nil || seo.name == "" {
		return &QorSEOSetting{}
	}
	seoSetting := &QorSEOSetting{}
	seoSettingOfParent := seo.parent.getFinalQorSEOSetting(locale, db)
	err := db.Where("name = ? and locale_code = ?", seo.name, locale).First(seoSetting).Error
	if err != nil {
		panic(err)
	}
	highPSetting := &seoSetting.Setting
	lowPSetting := &seoSettingOfParent.Setting
	mergeSetting(lowPSetting, highPSetting)
	if seoSetting.Variables == nil {
		seoSetting.Variables = make(Variables)
	}
	variables := seoSetting.Variables
	variablesOfParent := seoSettingOfParent.Variables
	for varName, val := range variablesOfParent {
		if _, isExist := variables[varName]; !isExist {
			variables[varName] = val
		}
	}
	return seoSetting
}

func (seo *SEO) getFinalContextVars() map[string]contextVariablesFunc {
	if seo == nil {
		return nil
	}
	if seo.finalContextVarsCache != nil {
		return seo.finalContextVarsCache
	} else {
		seo.finalContextVarsCache = make(map[string]contextVariablesFunc)
		for varName, varFunc := range seo.contextVars {
			seo.finalContextVarsCache[varName] = varFunc
		}
		contextVarsOfParent := seo.parent.getFinalContextVars()
		for varName, varFunc := range contextVarsOfParent {
			if _, isExist := seo.finalContextVarsCache[varName]; !isExist {
				seo.finalContextVarsCache[varName] = varFunc
			}
		}
		return seo.finalContextVarsCache
	}
}

func (seo *SEO) checkConflict(varName string, isContextVar bool) {
	node := seo
	if isContextVar {
		for node != nil {
			if _, isExist := node.settingVars[varName]; isExist {
				errMsg := fmt.Sprintf("There is already a setting variable named \"%v\" in %v. "+
					"Please use a different name", varName, node.name)
				panic(errMsg)
			}
			node = node.parent
		}
	} else {
		for node != nil {
			if _, isExist := node.contextVars[varName]; isExist {
				errMsg := fmt.Sprintf("There is already a context variable named \"%v\" in %v. "+
					"Please use a different name", varName, node.name)
				panic(errMsg)
			}
			node = node.parent
		}
	}
}

func mergeSetting(lowPSetting, highPSetting *Setting) {
	if highPSetting.Title == "" {
		highPSetting.Title = lowPSetting.Title
	}
	if highPSetting.Description == "" {
		highPSetting.Description = lowPSetting.Description
	}
	if highPSetting.Keywords == "" {
		highPSetting.Keywords = lowPSetting.Keywords
	}
	if highPSetting.OpenGraphTitle == "" {
		highPSetting.OpenGraphTitle = lowPSetting.OpenGraphTitle
	}
	if highPSetting.OpenGraphDescription == "" {
		highPSetting.OpenGraphDescription = lowPSetting.OpenGraphDescription
	}
	if highPSetting.OpenGraphURL == "" {
		highPSetting.OpenGraphURL = lowPSetting.OpenGraphURL
	}
	if highPSetting.OpenGraphType == "" {
		highPSetting.OpenGraphType = lowPSetting.OpenGraphType
	}
	if highPSetting.OpenGraphImageURL == "" {
		highPSetting.OpenGraphImageURL = lowPSetting.OpenGraphImageURL
	}
	if highPSetting.OpenGraphImageURL == "" {
		highPSetting.OpenGraphImageURL = lowPSetting.OpenGraphImageFromMediaLibrary.URL("og")
	}
	if len(highPSetting.OpenGraphMetadata) == 0 {
		highPSetting.OpenGraphMetadata = lowPSetting.OpenGraphMetadata
	}
}
