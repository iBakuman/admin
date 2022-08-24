package pagebuilder

import (
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"

	vx "github.com/goplaid/x/vuetifyx"

	"github.com/goplaid/web"
	"github.com/goplaid/x/perm"
	"github.com/goplaid/x/presets"
	"github.com/goplaid/x/presets/actions"
	. "github.com/goplaid/x/vuetify"
	"github.com/qor/qor5/publish"
	"github.com/sunfmin/reflectutils"
	h "github.com/theplant/htmlgo"
	"goji.io/pat"
	"gorm.io/gorm"
)

const (
	AddContainerDialogEvent          = "page_builder_AddContainerDialogEvent"
	AddContainerEvent                = "page_builder_AddContainerEvent"
	DeleteContainerConfirmationEvent = "page_builder_DeleteContainerConfirmationEvent"
	DeleteContainerEvent             = "page_builder_DeleteContainerEvent"
	MoveContainerEvent               = "page_builder_MoveContainerEvent"
	ToggleContainerVisibilityEvent   = "page_builder_ToggleContainerVisibilityEvent"
	MarkAsSharedContainerEvent       = "page_builder_MarkAsSharedContainerEvent"
	RenameCotainerDialogEvent        = "page_builder_RenameContainerDialogEvent"
	RenameContainerEvent             = "page_builder_RenameContainerEvent"

	paramPageID          = "pageID"
	paramPageVersion     = "pageVersion"
	paramContainerID     = "containerID"
	paramMoveResult      = "moveResult"
	paramContainerName   = "containerName"
	paramSharedContainer = "sharedContainer"
	paramModelID         = "modelID"
)

//go:embed dist
var box embed.FS

func ShadowDomComponentsPack() web.ComponentsPack {
	v, err := box.ReadFile("dist/shadow.min.js")
	if err != nil {
		panic(err)
	}
	return web.ComponentsPack(v)
}

func (b *Builder) Preview(ctx *web.EventContext) (r web.PageResponse, err error) {
	isTpl := ctx.R.FormValue("tpl") != ""
	id := ctx.R.FormValue("id")
	version := ctx.R.FormValue("version")
	ctx.Injector.HeadHTMLComponent("style", b.pageStyle, true)

	var comps []h.HTMLComponent
	var p *Page
	if isTpl {
		tpl := &Template{}
		err = b.db.First(tpl, "id = ?", id).Error
		if err != nil {
			return
		}
		p = tpl.Page()
		version = p.Version.Version
	} else {
		err = b.db.First(&p, "id = ? and version = ?", id, version).Error
		if err != nil {
			return
		}
	}
	comps, err = b.renderContainers(ctx, p.ID, p.GetVersion(), false)
	if err != nil {
		return
	}

	r.PageTitle = p.Title
	r.Body = h.Components(comps...)
	if b.pageLayoutFunc != nil {
		input := &PageLayoutInput{
			IsPreview: true,
			Page:      p,
		}
		r.Body = b.pageLayoutFunc(h.Components(comps...), input, ctx)
	}
	return
}

func (b *Builder) Editor(ctx *web.EventContext) (r web.PageResponse, err error) {
	isTpl := ctx.R.FormValue("tpl") != ""
	id := pat.Param(ctx.R, "id")
	version := ctx.R.FormValue("version")
	var comps []h.HTMLComponent
	var body h.HTMLComponent
	var containerList h.HTMLComponent
	var device string
	var p *Page
	var previewHref string
	deviceQueries := url.Values{}
	if isTpl {
		tpl := &Template{}
		err = b.db.First(tpl, "id = ?", id).Error
		if err != nil {
			return
		}
		p = tpl.Page()
		version = p.Version.Version
		previewHref = fmt.Sprintf("/preview?id=%s&tpl=1", id)
		deviceQueries.Add("tpl", "1")
	} else {
		err = b.db.First(&p, "id = ? and version = ?", id, version).Error
		if err != nil {
			return
		}
		previewHref = fmt.Sprintf("/preview?id=%s&version=%s", id, version)
		deviceQueries.Add("version", version)
	}
	if p.GetStatus() == publish.StatusDraft {
		comps, err = b.renderContainers(ctx, p.ID, p.GetVersion(), true)
		if err != nil {
			return
		}
		r.PageTitle = fmt.Sprintf("Editor for %s: %s", id, p.Title)
		device, _ = b.getDevice(ctx)
		body = h.Components(comps...)
		if b.pageLayoutFunc != nil {
			input := &PageLayoutInput{
				IsEditor: true,
				Page:     p,
			}
			body = b.pageLayoutFunc(h.Components(comps...), input, ctx)

			containerList, err = b.renderContainersList(ctx, p.ID, p.GetVersion())
			if err != nil {
				return
			}
		}
	} else {
		body = h.Text(perm.PermissionDenied.Error())
	}
	r.Body = h.Components(
		VAppBar(
			VSpacer(),

			VBtn("").Icon(true).Children(
				VIcon("phone_iphone"),
			).Attr("@click", web.Plaid().Queries(deviceQueries).Query("device", "phone").PushState(true).Go()).
				Class("mr-10").InputValue(device == "phone"),

			VBtn("").Icon(true).Children(
				VIcon("tablet_mac"),
			).Attr("@click", web.Plaid().Queries(deviceQueries).Query("device", "tablet").PushState(true).Go()).
				Class("mr-10").InputValue(device == "tablet"),

			VBtn("").Icon(true).Children(
				VIcon("laptop_mac"),
			).Attr("@click", web.Plaid().Queries(deviceQueries).Query("device", "laptop").PushState(true).Go()).
				InputValue(device == "laptop"),

			VSpacer(),

			VBtn("Preview").Text(true).Href(b.prefix+previewHref).Target("_blank"),
			VAppBarNavIcon().On("click.stop", "vars.navDrawer = !vars.navDrawer"),
		).Dark(true).
			Color("primary").
			App(true),

		VMain(
			VContainer(body).Attr("v-keep-scroll", "true").
				Class("mt-6").
				Fluid(true),
			VNavigationDrawer(containerList).
				App(true).
				Right(true).
				Fixed(true).
				Value(true).
				Width(420).
				Attr("v-model", "vars.navDrawer").
				Attr(web.InitContextVars, `{navDrawer: null}`),
		),
	)

	return
}

func (b *Builder) getDevice(ctx *web.EventContext) (device string, style string) {
	device = ctx.R.FormValue("device")
	if len(device) == 0 {
		device = "phone"
	}

	switch device {
	case "phone":
		style = "width: 414px;"
	case "tablet":
		style = "width: 768px;"
	case "laptop":
		style = "width: 1264px;"
	}

	return
}

func (b *Builder) renderContainers(ctx *web.EventContext, pageID uint, pageVersion string, isEditor bool) (r []h.HTMLComponent, err error) {
	var cons []*Container
	err = b.db.Order("display_order ASC").Find(&cons, "page_id = ? AND page_version = ?", pageID, pageVersion).Error
	if err != nil {
		return
	}

	cbs := b.getContainerBuilders(cons)

	device, width := b.getDevice(ctx)
	for _, ec := range cbs {
		if ec.container.Hidden {
			continue
		}
		obj := ec.builder.NewModel()
		err = b.db.FirstOrCreate(obj, "id = ?", ec.container.ModelID).Error
		if err != nil {
			return
		}

		input := RenderInput{
			IsEditor: isEditor,
			Device:   device,
		}
		pure := ec.builder.renderFunc(obj, &input, ctx)
		r = append(r, pure)
	}
	if isEditor {
		containerContent := h.HTML(
			h.Head(
				b.pageStyle,
				h.RawHTML(`<link rel="stylesheet" href="https://fonts.googleapis.com/icon?family=Material+Icons">`),
				h.Style(`
	.wrapper-shadow {
		position:absolute;
		width: 100%; 
		height: 100%;
		z-index:9999; 
		background: rgba(81, 193, 226, 0.25);
		opacity: 0;
		top: 0;
		left: 0;
	}
	.wrapper-shadow button{
		position:absolute;
		top: 0;
		right: 0;
    	line-height: 1;
		font-size: 0;
		border: 2px outset #767676;
	}
	.wrapper-shadow:hover {
		cursor: pointer;
		opacity: 1;
    }`),
				h.Script(``),
			),
			h.Body(
				h.Components(r...),
			),
		)
		iframe := VRow(
			h.Div(
				h.RawHTML(fmt.Sprintf("<iframe frameborder='0' scrolling='no' srcdoc=\"%s\" "+
					"@load='$event.target.style.height=$event.target.contentWindow.document.body.parentElement.offsetHeight+\"px\";'"+
					"style='width:100%%; display:block; border:none; padding:0; margin:0;'></iframe>",
					strings.ReplaceAll(
						h.MustString(containerContent, ctx.R.Context()),
						"\"",
						"&quot;"),
				)),
			).Class("page-builder-container mx-auto").Attr("style", width),
		)

		r = []h.HTMLComponent{iframe}
	}
	return
}

type ContainerSorterItem struct {
	Index          int    `json:"index"`
	Label          string `json:"label"`
	ModelName      string `json:"model_name"`
	ModelID        string `json:"model_id"`
	DisplayName    string `json:"display_name"`
	ContainerID    string `json:"container_id"`
	URL            string `json:"url"`
	Shared         bool   `json:"shared"`
	VisibilityIcon string `json:"visibility_icon"`
}

type ContainerSorter struct {
	Items []ContainerSorterItem `json:"items"`
}

func (b *Builder) renderContainersList(ctx *web.EventContext, pageID uint, pageVersion string) (r h.HTMLComponent, err error) {
	var cons []*Container
	err = b.db.Order("display_order ASC").Find(&cons, "page_id = ? AND page_version = ?", pageID, pageVersion).Error
	if err != nil {
		return
	}

	var sorterData ContainerSorter
	for i, c := range cons {
		vicon := "visibility"
		if c.Hidden {
			vicon = "visibility_off"
		}
		sorterData.Items = append(sorterData.Items,
			ContainerSorterItem{
				Index:          i,
				Label:          c.DisplayName,
				ModelName:      c.ModelName,
				ModelID:        strconv.Itoa(int(c.ModelID)),
				DisplayName:    c.DisplayName,
				ContainerID:    strconv.Itoa(int(c.ID)),
				URL:            b.ContainerByName(c.ModelName).mb.Info().ListingHref(),
				Shared:         c.Shared,
				VisibilityIcon: vicon,
			},
		)
	}

	r = web.Scope(
		VAppBar(
			VToolbarTitle("").Class("pl-2").
				Children(h.Text("Containers")),
			VSpacer(),
			//VBtn("").Icon(true).Children(
			//	VIcon("close"),
			//).Attr("@click.stop", "vars.presetsRightDrawer = false"),
		).Color("white").Elevation(0).Dense(true),

		VSheet(
			VCard(
				h.Tag("vx-draggable").
					Attr("v-model", "locals.items", "handle", ".handle", "animation", "300").
					Attr("@end", web.Plaid().
						EventFunc(MoveContainerEvent).
						FieldValue(paramMoveResult, web.Var("JSON.stringify(locals.items)")).
						Go()).Children(
					//VList(
					h.Div(
						VListItem(
							VListItemContent(
								VListItemTitle(h.Text("{{item.label}}")).Attr(":style", "[item.shared ? {'color':'green'}:{}]"),
							),
							VListItemIcon(VBtn("").Icon(true).Children(VIcon("edit"))).Attr("@click",
								web.Plaid().
									URL(web.Var("item.url")).
									EventFunc(actions.Edit).
									Query(presets.ParamOverlay, actions.Dialog).
									Query(presets.ParamID, web.Var("item.model_id")).
									Go(),
							).Class("my-2"),
							VListItemIcon(VBtn("").Icon(true).Children(VIcon("{{item.visibility_icon}}"))).Attr("@click",
								web.Plaid().
									EventFunc(ToggleContainerVisibilityEvent).
									Query(paramContainerID, web.Var("item.container_id")).
									Go(),
							).Class("my-2"),
							VListItemIcon(VBtn("").Icon(true).Children(VIcon("reorder"))).Class("handle my-2"),
							VMenu(
								web.Slot(
									VBtn("").Children(
										VIcon("more_horiz"),
									).Attr("v-on", "on").Text(true).Fab(true).Small(true),
								).Name("activator").Scope("{ on }"),

								VList(
									VListItem(
										VListItemIcon(VIcon("edit_note")).Class("pl-0 mr-2"),
										VListItemTitle(h.Text("Rename")),
									).Attr("@click",
										web.Plaid().
											EventFunc(RenameCotainerDialogEvent).
											Query(paramContainerID, web.Var("item.container_id")).
											Query(paramContainerName, web.Var("item.display_name")).
											Query(presets.ParamOverlay, actions.Dialog).
											Go(),
									),
									VListItem(
										VListItemIcon(VIcon("delete")).Class("pl-0 mr-2"),
										VListItemTitle(h.Text("Delete")),
									).Attr("@click", web.Plaid().
										EventFunc(DeleteContainerConfirmationEvent).
										Query(paramContainerID, web.Var("item.container_id")).
										Query(paramContainerName, web.Var("item.display_name")).
										Go(),
									),
									VListItem(
										VListItemIcon(VIcon("share")).Class("pl-0 mr-2"),
										VListItemTitle(h.Text("Mark As Shared Container")),
									).Attr("@click",
										web.Plaid().
											EventFunc(MarkAsSharedContainerEvent).
											Query(paramContainerID, web.Var("item.container_id")).
											Go(),
									).Attr("v-if", "!item.shared"),
								).Dense(true),
							).Left(true),
						),
						VDivider().Attr("v-if", "index < locals.items.length "),
					).Attr("v-for", "(item, index) in locals.items", ":key", "item.index"),
					VListItem(
						VListItemIcon(VIcon("add").Color("primary")).Class("ma-4"),
						VListItemTitle(VBtn("Add Containers").Color("primary").Text(true)),
					).Attr("@click",
						web.Plaid().
							EventFunc(AddContainerDialogEvent).
							Query(paramPageID, pageID).
							Query(paramPageVersion, pageVersion).
							Query(presets.ParamOverlay, actions.Dialog).
							Go(),
					),
					//).Class("py-0"),
				),
			),
		).Class("pa-4 pt-2"),
	).Init(h.JSONString(sorterData)).VSlot("{ locals }")
	return
}

func (b *Builder) AddContainer(ctx *web.EventContext) (r web.EventResponse, err error) {
	pageID := ctx.QueryAsInt(paramPageID)
	pageVersion := ctx.R.FormValue(paramPageVersion)
	containerName := ctx.R.FormValue(paramContainerName)
	sharedContainer := ctx.R.FormValue(paramSharedContainer)
	modelID := ctx.QueryAsInt(paramModelID)
	var newModelID uint
	if sharedContainer == "true" {
		err = b.AddSharedContainerToPage(pageID, pageVersion, containerName, uint(modelID))
		r.PushState = web.Location(url.Values{})
	} else {
		newModelID, err = b.AddContainerToPage(pageID, pageVersion, containerName)
		r.VarsScript = web.Plaid().
			URL(b.ContainerByName(containerName).mb.Info().ListingHref()).
			EventFunc(actions.Edit).
			Query(presets.ParamOverlay, actions.Dialog).
			Query(presets.ParamID, fmt.Sprint(newModelID)).
			Go()
	}

	return
}

func (b *Builder) MoveContainer(ctx *web.EventContext) (r web.EventResponse, err error) {
	moveResult := ctx.R.FormValue(paramMoveResult)

	var result []ContainerSorterItem
	err = json.Unmarshal([]byte(moveResult), &result)
	if err != nil {
		return
	}
	err = b.db.Transaction(func(tx *gorm.DB) (inerr error) {
		for i, r := range result {
			if inerr = tx.Model(&Container{}).Where("id = ?", r.ContainerID).Update("display_order", i+1).Error; inerr != nil {
				return
			}
		}
		return
	})

	r.PushState = web.Location(url.Values{})
	return
}
func (b *Builder) ToggleContainerVisibility(ctx *web.EventContext) (r web.EventResponse, err error) {
	containerID := ctx.R.FormValue(paramContainerID)
	err = b.db.Exec("UPDATE page_builder_containers SET hidden = NOT(COALESCE(hidden,FALSE)) WHERE id = ?", containerID).Error

	r.PushState = web.Location(url.Values{})
	return
}

func (b *Builder) DeleteContainerConfirmation(ctx *web.EventContext) (r web.EventResponse, err error) {
	containerID := ctx.R.FormValue(paramContainerID)
	containerName := ctx.R.FormValue(paramContainerName)

	r.UpdatePortals = append(r.UpdatePortals, &web.PortalUpdate{
		Name: presets.DeleteConfirmPortalName,
		Body: VDialog(
			VCard(
				VCardTitle(h.Text(fmt.Sprintf("Are you sure you want to delete %s?", containerName))),
				VCardActions(
					VSpacer(),
					VBtn("Cancel").
						Depressed(true).
						Class("ml-2").
						On("click", "vars.deleteConfirmation = false"),

					VBtn("Delete").
						Color("primary").
						Depressed(true).
						Dark(true).
						Attr("@click", web.Plaid().
							EventFunc(DeleteContainerEvent).
							Query(paramContainerID, containerID).
							Go()),
				),
			),
		).MaxWidth("600px").
			Attr("v-model", "vars.deleteConfirmation").
			Attr(web.InitContextVars, `{deleteConfirmation: false}`),
	})

	r.VarsScript = "setTimeout(function(){ vars.deleteConfirmation = true }, 100)"
	return
}

func (b *Builder) DeleteContainer(ctx *web.EventContext) (r web.EventResponse, err error) {
	containerID := ctx.QueryAsInt(paramContainerID)

	err = b.db.Delete(&Container{}, "id = ?", containerID).Error
	if err != nil {
		return
	}
	r.PushState = web.Location(url.Values{})
	return
}

func (b *Builder) AddContainerToPage(pageID int, pageVersion, containerName string) (modelID uint, err error) {
	model := b.ContainerByName(containerName).NewModel()
	var dc DemoContainer
	b.db.Where("model_name = ?", containerName).First(&dc)
	if dc.ID != 0 && dc.ModelID != 0 {
		b.db.Where("id = ?", dc.ModelID).First(model)
		reflectutils.Set(model, "ID", uint(0))
	}

	err = b.db.Create(model).Error
	if err != nil {
		return
	}

	var maxOrder sql.NullFloat64
	err = b.db.Model(&Container{}).Select("MAX(display_order)").Where("page_id = ? and page_version = ?", pageID, pageVersion).Scan(&maxOrder).Error
	if err != nil {
		return
	}

	modelID = reflectutils.MustGet(model, "ID").(uint)
	err = b.db.Create(&Container{
		PageID:       uint(pageID),
		PageVersion:  pageVersion,
		ModelName:    containerName,
		DisplayName:  containerName,
		ModelID:      modelID,
		DisplayOrder: maxOrder.Float64 + 1,
	}).Error
	if err != nil {
		return
	}
	return
}

func (b *Builder) AddSharedContainerToPage(pageID int, pageVersion, containerName string, modelID uint) (err error) {
	var c Container
	err = b.db.First(&c, "model_name = ? AND model_id = ? AND shared = true", containerName, modelID).Error
	if err != nil {
		return
	}
	var maxOrder sql.NullFloat64
	err = b.db.Model(&Container{}).Select("MAX(display_order)").Where("page_id = ? and page_version = ?", pageID, pageVersion).Scan(&maxOrder).Error
	if err != nil {
		return
	}

	err = b.db.Create(&Container{
		PageID:       uint(pageID),
		PageVersion:  pageVersion,
		ModelName:    containerName,
		DisplayName:  c.DisplayName,
		ModelID:      modelID,
		Shared:       true,
		DisplayOrder: maxOrder.Float64 + 1,
	}).Error
	if err != nil {
		return
	}
	return
}

func (b *Builder) copyContainersToNewPageVersion(db *gorm.DB, pageID int, oldPageVersion, newPageVersion string) (err error) {
	return b.copyContainersToAnotherPage(db, pageID, oldPageVersion, pageID, newPageVersion)
}

func (b *Builder) copyContainersToAnotherPage(db *gorm.DB, pageID int, pageVersion string, toPageID int, toPageVersion string) (err error) {
	var cons []*Container
	err = db.Order("display_order ASC").Find(&cons, "page_id = ? AND page_version = ?", pageID, pageVersion).Error
	if err != nil {
		return
	}

	for _, c := range cons {
		newModelID := c.ModelID
		if !c.Shared {
			model := b.ContainerByName(c.ModelName).NewModel()
			if err = db.First(model, "id = ?", c.ModelID).Error; err != nil {
				return
			}
			if err = reflectutils.Set(model, "ID", uint(0)); err != nil {
				return
			}
			if err = db.Create(model).Error; err != nil {
				return
			}
			newModelID = reflectutils.MustGet(model, "ID").(uint)
		}

		if err = db.Create(&Container{
			PageID:       uint(toPageID),
			PageVersion:  toPageVersion,
			ModelName:    c.ModelName,
			DisplayName:  c.DisplayName,
			ModelID:      newModelID,
			DisplayOrder: c.DisplayOrder,
			Shared:       c.Shared,
		}).Error; err != nil {
			return
		}
	}
	return
}

func (b *Builder) MarkAsSharedContainer(ctx *web.EventContext) (r web.EventResponse, err error) {
	containerID := ctx.QueryAsInt(paramContainerID)
	err = b.db.Model(&Container{}).Where("id = ?", containerID).Update("shared", true).Error
	if err != nil {
		return
	}
	r.PushState = web.Location(url.Values{})
	return
}

func (b *Builder) RenameContainer(ctx *web.EventContext) (r web.EventResponse, err error) {
	containerID := ctx.QueryAsInt(paramContainerID)
	name := ctx.R.FormValue("DisplayName")
	var c Container
	err = b.db.First(&c, "id = ?  ", containerID).Error
	if err != nil {
		return
	}
	if c.Shared {
		err = b.db.Model(&Container{}).Where("model_name = ? AND model_id = ?", c.ModelName, c.ModelID).Update("display_name", name).Error
		if err != nil {
			return
		}
	} else {
		err = b.db.Model(&Container{}).Where("id = ?", containerID).Update("display_name", name).Error
		if err != nil {
			return
		}
	}

	r.PushState = web.Location(url.Values{})
	return
}

func (b *Builder) RenameContainerDialog(ctx *web.EventContext) (r web.EventResponse, err error) {
	containerID := ctx.QueryAsInt(paramContainerID)
	name := ctx.R.FormValue(paramContainerName)
	okAction := web.Plaid().EventFunc(RenameContainerEvent).Query(paramContainerID, containerID).Go()
	r.UpdatePortals = append(r.UpdatePortals, &web.PortalUpdate{
		Name: dialogPortalName,
		Body: web.Scope(
			VDialog(
				VCard(
					VCardTitle(h.Text("Rename")),
					VCardText(
						VTextField().FieldName("DisplayName").Value(name),
					),
					VCardActions(
						VSpacer(),
						VBtn("Cancel").
							Depressed(true).
							Class("ml-2").
							On("click", "locals.renameDialog = false"),

						VBtn("OK").
							Color("primary").
							Depressed(true).
							Dark(true).
							Attr("@click", okAction),
					),
				),
			).MaxWidth("400px").
				Attr("v-model", "locals.renameDialog"),
		).Init("{renameDialog:true}").VSlot("{locals}"),
	})
	return
}

func (b *Builder) AddContainerDialog(ctx *web.EventContext) (r web.EventResponse, err error) {
	pageID := ctx.QueryAsInt(paramPageID)
	pageVersion := ctx.R.FormValue(paramPageVersion)
	// okAction := web.Plaid().EventFunc(RenameContainerEvent).Query(paramContainerID, containerID).Go()

	var containers []h.HTMLComponent
	for _, builder := range b.containerBuilders {
		cover := builder.cover
		if cover == "" {
			cover = path.Join(b.prefix, b.imagesPrefix, strings.ReplaceAll(builder.name, " ", "")+".png")
		}
		containers = append(containers,
			VCol(
				VCard(
					VImg().Src(cover).Height(200),
					VCardActions(
						VCardTitle(h.Text(builder.name)),
						VSpacer(),
						VBtn("Select").
							Text(true).
							Color("primary").Attr("@click",
							"locals.addContainerDialog = false;"+web.Plaid().
								EventFunc(AddContainerEvent).
								Query(paramPageID, pageID).
								Query(paramPageVersion, pageVersion).
								Query(paramContainerName, builder.name).
								Go(),
						),
					),
				),
			).Cols(4),
		)
	}

	var cons []*Container
	err = b.db.Select("display_name,model_name,model_id").Where("shared = true").Group("display_name,model_name,model_id").Find(&cons).Error
	if err != nil {
		return
	}

	var sharedContainers []h.HTMLComponent
	for _, sharedC := range cons {
		c := b.ContainerByName(sharedC.ModelName)
		cover := c.cover
		if cover == "" {
			cover = path.Join(b.prefix, b.imagesPrefix, strings.ReplaceAll(c.name, " ", "")+".png")
		}
		sharedContainers = append(sharedContainers,
			VCol(
				VCard(
					VImg().Src(cover).Height(200),
					VCardActions(
						VCardTitle(h.Text(sharedC.DisplayName)),
						VSpacer(),
						VBtn("Select").
							Text(true).
							Color("primary").Attr("@click",
							"locals.addContainerDialog = false;"+web.Plaid().
								EventFunc(AddContainerEvent).
								Query(paramPageID, pageID).
								Query(paramPageVersion, pageVersion).
								Query(paramContainerName, sharedC.ModelName).
								Query(paramModelID, sharedC.ModelID).
								Query(paramSharedContainer, "true").
								Go(),
						),
					),
				),
			).Cols(4),
		)
	}

	r.UpdatePortals = append(r.UpdatePortals, &web.PortalUpdate{
		Name: dialogPortalName,
		Body: web.Scope(
			VDialog(
				VTabs(
					VTab(h.Text("New")),
					VTabItem(
						VSheet(
							VContainer(
								VRow(
									containers...,
								),
							),
						),
					).Attr("style", "overflow-y: scroll; overflow-x: hidden; height: 610px;"),
					VTab(h.Text("Shared")),
					VTabItem(
						VSheet(
							VContainer(
								VRow(
									sharedContainers...,
								),
							),
						),
					).Attr("style", "overflow-y: scroll; overflow-x: hidden; height: 610px;"),
				),
			).Width("1200px").Attr("v-model", "locals.addContainerDialog"),
		).Init("{addContainerDialog:true}").VSlot("{locals}"),
	})

	return
}

type editorContainer struct {
	builder   *ContainerBuilder
	container *Container
}

func (b *Builder) getContainerBuilders(cs []*Container) (r []*editorContainer) {
	for _, c := range cs {
		for _, cb := range b.containerBuilders {
			if cb.name == c.ModelName {
				r = append(r, &editorContainer{
					builder:   cb,
					container: c,
				})
			}
		}
	}
	return
}

const (
	dialogPortalName = "pagebuilder_DialogPortalName"
)

func (b *Builder) pageEditorLayout(in web.PageFunc, config *presets.LayoutConfig) (out web.PageFunc) {
	return func(ctx *web.EventContext) (pr web.PageResponse, err error) {

		ctx.Injector.HeadHTML(strings.Replace(`
			<link rel="stylesheet" href="https://fonts.googleapis.com/css?family=Roboto+Mono">
			<link rel="stylesheet" href="https://fonts.googleapis.com/css?family=Roboto:300,400,500">
			<link rel="stylesheet" href="https://fonts.googleapis.com/icon?family=Material+Icons">
			<link rel="stylesheet" href="{{prefix}}/assets/main.css">
			<script src='{{prefix}}/assets/vue.js'></script>

<style>
	.page-builder-container {
		overflow: hidden;
		box-shadow: -10px 0px 13px -7px rgba(0,0,0,.3), 10px 0px 13px -7px rgba(0,0,0,.18), 5px 0px 15px 5px rgba(0,0,0,.12);	
}
</style>

			<style>
				[v-cloak] {
					display: none;
				}
			</style>
		`, "{{prefix}}", b.prefix, -1))

		b.ps.InjectExtraAssets(ctx)

		if len(os.Getenv("DEV_PRESETS")) > 0 {
			ctx.Injector.TailHTML(`
<script src='http://localhost:3080/js/chunk-vendors.js'></script>
<script src='http://localhost:3080/js/app.js'></script>
<script src='http://localhost:3100/js/chunk-vendors.js'></script>
<script src='http://localhost:3100/js/app.js'></script>
			`)

		} else {
			ctx.Injector.TailHTML(strings.Replace(`
			<script src='{{prefix}}/assets/main.js'></script>
			`, "{{prefix}}", b.prefix, -1))
		}

		var innerPr web.PageResponse
		innerPr, err = in(ctx)
		if err != nil {
			panic(err)
		}

		pr.PageTitle = fmt.Sprintf("%s - %s", innerPr.PageTitle, "Page Builder")
		pr.Body = VApp(

			web.Portal().Name(presets.RightDrawerPortalName),
			web.Portal().Name(presets.DialogPortalName),
			web.Portal().Name(presets.DeleteConfirmPortalName),
			web.Portal().Name(dialogPortalName),
			vx.VXMessageListener().ListenFunc(fmt.Sprintf(`function(e){
let arr = e.data.split('_');
if (arr.length!=2) {
console.log(arr);
return 
}
$plaid().vars(vars).form(plaidForm).url("%s/"+arr[0]).eventFunc("presets_Edit").query("overlay", "dialog").query("id", arr[1]).go();
}`, b.prefix)),

			innerPr.Body.(h.HTMLComponent),
		).Id("vt-app").Attr(web.InitContextVars, `{presetsRightDrawer: false, presetsDialog: false, dialogPortalName: false}`)

		return
	}
}
