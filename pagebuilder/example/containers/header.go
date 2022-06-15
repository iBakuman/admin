package containers

import (
	"fmt"

	"github.com/goplaid/web"
	"github.com/goplaid/x/presets"
	"github.com/goplaid/x/vuetify"
	"github.com/qor/qor5/pagebuilder"
	h "github.com/theplant/htmlgo"
)

type WebHeader struct {
	ID    uint
	Color string
}

func (*WebHeader) TableName() string {
	return "container_headers"
}

func RegisterHeader(pb *pagebuilder.Builder) {
	header := pb.RegisterContainer("Header").
		RenderFunc(func(obj interface{}, ctx *web.EventContext) h.HTMLComponent {
			header := obj.(*WebHeader)
			return HeaderTemplate(header.Color)
		})

	ed := header.Model(&WebHeader{}).Editing("Color")
	ed.Field("Color").ComponentFunc(func(obj interface{}, field *presets.FieldContext, ctx *web.EventContext) h.HTMLComponent {
		return vuetify.VSelect().
			Items([]string{"black", "white"}).
			Value(field.Value(obj)).
			Label(field.Label).
			FieldName(field.FormKey)
	})
}

func HeaderTemplate(color string) h.HTMLComponent {
	return h.RawHTML(fmt.Sprintf(`<header data-navigation-color="%s" class="container-instance container-header">
<div class="container-wrapper">
<a href="/" class="container-header-logo"><svg viewBox="0 0 29 30" fill="none" xmlns="http://www.w3.org/2000/svg"><path fill-rule="evenodd" clip-rule="evenodd" d="M14.399 10.054V0L0 10.054V29.73h28.792V0L14.4 10.054z" fill="currentColor"><title>The Plant</title></path></svg></a>
<ul data-list-unset="true" class="container-header-links">
<li>
<a href="/what-we-do/">What we do</a>
</li>
<li>
<a href="/projects/">Projects</a>
</li>
<li>
<a href="/why-clients-choose-us/">Why clients choose us</a>
</li>
<li>
<a href="/our-company/">Our company</a>
</li>
</ul>
<button class="container-header-menu">
<span class="container-header-menu-icon"></span>
</button>
</div>
</header>`, color))
}