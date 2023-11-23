package seo

import "reflect"

// SEO represents a seo object for a page
// @snippet_begin(SeoDefinition)
type SEO struct {
	// The parent field represents the upper-level SEO for this SEO.
	// When certain fields are not presents in the current SEO,
	// the values for these fields will be retrieved from the
	// parent SEO.
	// NOTE: The parent of Global SEO is nil.
	parent *SEO

	// children records SEOs with higher priority than the current SEO.
	// when certain fields are missing in these SEOs, they can refer to
	// the current SEO to obtain these fields
	children []*SEO

	// The name field is a unique identifier and cannot be duplicated.
	name             string
	modelTyp         reflect.Type
	contextVariables map[string]contextVariablesFunc // fetch context variables from request
	settingVariables interface{}                     // fetch setting variables from db
}

// @snippet_end

// AddChildren Set the parent of each child in the children to current seo.
// Usage:
// collection.RegisterSEO(father).AddChildren(
//
//	collection.RegisterSEO(son1),
//	collection.RegisterSEO(son2),
//	collection.RegisterSEO(son3),
//
// )
func (seo *SEO) AddChildren(children ...*SEO) *SEO {
	if seo == nil {
		return seo
	}

	for _, child := range children {
		// prevent child is self or nil
		if child == seo || child == nil {
			continue
		}
		child.SetParent(seo)
	}
	return seo
}

// SetParent sets the parent of the seo to newParent
// if newParent is already the parent of seo, it returns itself directly.
// if newParent
func (seo *SEO) SetParent(newParent *SEO) *SEO {
	// 1. seo is nil
	// 2. newParent is already the parent node of seo
	if seo == nil || seo.parent == newParent {
		return seo
	}

	// Check if the new parent is a descendant node of the current seo node.
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

	// remove current seo node from oldParent
	oldParent := seo.parent
	if oldParent != nil {
		for i, childOfOldParent := range oldParent.children {
			if childOfOldParent == seo {
				oldParent.children = append(oldParent.children[:i], oldParent.children[i+1:]...)
				break
			}
		}
	}
	// add current seo node to newParent
	seo.parent = newParent
	if newParent != nil {
		newParent.children = append(newParent.children, seo)
	}
	return seo
}

// RemoveSelf removes itself from the seo tree.
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
			// Note: Do not use "child.parent = seo.parent"
			child.SetParent(seo.parent)
		}
	}
	return seo
}

func (seo *SEO) SetModel(model interface{}) *SEO {
	seo.modelTyp = reflect.Indirect(reflect.ValueOf(model)).Type()
	return seo
}

// SetName set seo name
func (seo *SEO) SetName(name string) *SEO {
	seo.name = name
	return seo
}

// RegisterContextVariables register context variables. the registered variables will be rendered to the page
func (seo *SEO) RegisterContextVariables(key string, f contextVariablesFunc) *SEO {
	if seo.contextVariables == nil {
		seo.contextVariables = map[string]contextVariablesFunc{}
	}
	seo.contextVariables[key] = f
	return seo
}

// RegisterSettingVaribles register a setting variable
func (seo *SEO) RegisterSettingVaribles(setting interface{}) *SEO {
	seo.settingVariables = setting
	return seo
}

func NewSEO(obj interface{}) *SEO {
	return &SEO{name: GetSEOName(obj)}
}
