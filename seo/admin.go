package seo

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/qor5/admin/l10n"
	"github.com/qor5/admin/media"
	"github.com/qor5/admin/media/media_library"
	"github.com/qor5/admin/media/views"
	"github.com/qor5/admin/presets"
	. "github.com/qor5/ui/vuetify"
	"github.com/qor5/web"
	"github.com/qor5/x/i18n"
	"github.com/qor5/x/perm"
	"github.com/sunfmin/reflectutils"
	h "github.com/theplant/htmlgo"
	"golang.org/x/text/language"
	"gorm.io/gorm"
)

const (
	saveEvent                 = "seo_save_collection"
	I18nSeoKey i18n.ModuleKey = "I18nSeoKey"
)

var permVerifier *perm.Verifier

func (b *Builder) Configure(pb *presets.Builder, db *gorm.DB) (pm *presets.ModelBuilder) {
	if err := db.AutoMigrate(&QorSEOSetting{}); err != nil {
		panic(err)
	}

	if GlobalDB == nil {
		GlobalDB = db
	}

	pb.GetWebBuilder().RegisterEventFunc(saveEvent, b.save)

	pm = pb.Model(&QorSEOSetting{}).PrimaryField("Name").Label("SEO")
	// Configure Listing Page
	{

		pml := pm.Listing("Name", "CreatedAt")
		// disable new btn globally, no one can add new seo record after server start up.
		pml.NewButtonFunc(func(ctx *web.EventContext) h.HTMLComponent {
			return nil
		})

		// Remove the menu from each line
		pml.RowMenu().Empty()

		// Configure the indentation for Name field to display hierarchy.
		pml.Field("Name").ComponentFunc(func(obj interface{}, field *presets.FieldContext, ctx *web.EventContext) h.HTMLComponent {
			seo := obj.(*QorSEOSetting)
			icon := "folder"
			priority := b.GetSEOPriority(seo.Name)
			// listingOrders := collection.SortSEOs()
			return h.Td(
				h.Div(
					VIcon(icon).Small(true).Class("mb-1"),
					h.Text(seo.Name),
				).Style(fmt.Sprintf("padding-left: %dpx;", 32*(priority-1))),
			)
		})

		oldSearcher := pml.Searcher
		pml.SearchFunc(func(model interface{}, params *presets.SearchParams, ctx *web.EventContext) (r interface{}, totalCount int, err error) {
			// msgr      := i18n.MustGetModuleMessages(ctx.R, I18nSeoKey, Messages_en_US).(*Messages)
			db := b.getDBFromContext(ctx.R.Context())
			locale, _ := l10n.IsLocalizableFromCtx(ctx.R.Context())
			for name := range b.registeredSEO {
				var modelSetting QorSEOSetting
				err := db.Where("name = ? AND locale_code = ?", name, locale).First(&modelSetting).Error
				if errors.Is(err, gorm.ErrRecordNotFound) {
					modelSetting.Name = name
					modelSetting.SetLocale(locale)
					if err := db.Save(&modelSetting).Error; err != nil {
						panic(err)
					}
				}
			}
			r, totalCount, err = oldSearcher(model, params, ctx)
			b.SortSEOs(r.([]*QorSEOSetting))
			return
		})
	}
	// pm.Editing("Setting").Field("Setting").ComponentFunc(
	// 	func(obj interface{}, field *presets.FieldContext, ctx *web.EventContext) h.HTMLComponent {
	// 		return collection.vseo(fieldPrefix, seo, &setting, ctx.R),
	// 	},
	// )
	// pm.Listing().PageFunc(collection.pageFunc)
	// pm = b.Model(collection.settingModel).Label("SEO")
	pb.FieldDefaults(presets.WRITE).
		FieldType(Setting{}).
		ComponentFunc(b.EditingComponentFunc).
		SetterFunc(EditSetterFunc)

	pb.I18n().
		RegisterForModule(language.English, I18nSeoKey, Messages_en_US).
		RegisterForModule(language.SimplifiedChinese, I18nSeoKey, Messages_zh_CN)

	pb.ExtraAsset("/vue-seo.js", "text/javascript", SeoJSComponentsPack())
	permVerifier = perm.NewVerifier("seo", pb.GetPermission())
	return
}

func EditSetterFunc(obj interface{}, field *presets.FieldContext, ctx *web.EventContext) (err error) {
	var setting Setting
	var mediaBox = media_library.MediaBox{}
	for fieldWithPrefix := range ctx.R.Form {
		// make sure OpenGraphImageFromMediaLibrary.Description set after OpenGraphImageFromMediaLibrary.Values
		if fieldWithPrefix == fmt.Sprintf("%s.%s", field.Name, "OpenGraphImageFromMediaLibrary.Values") {
			err = mediaBox.Scan(ctx.R.FormValue(fieldWithPrefix))
			if err != nil {
				return
			}
			break
		}
	}
	for fieldWithPrefix := range ctx.R.Form {
		if strings.HasPrefix(fieldWithPrefix, fmt.Sprintf("%s.%s", field.Name, "OpenGraphImageFromMediaLibrary")) {
			if fieldWithPrefix == fmt.Sprintf("%s.%s", field.Name, "OpenGraphImageFromMediaLibrary.Description") {
				mediaBox.Description = ctx.R.Form.Get(fieldWithPrefix)
				reflectutils.Set(&setting, "OpenGraphImageFromMediaLibrary", mediaBox)
			}
			continue
		}
		if fieldWithPrefix == fmt.Sprintf("%s.%s", field.Name, "OpenGraphMetadataString") {
			metadata := GetOpenGraphMetadata(ctx.R.Form.Get(fieldWithPrefix))
			reflectutils.Set(&setting, "OpenGraphMetadata", metadata)
			continue
		}
		if strings.HasPrefix(fieldWithPrefix, fmt.Sprintf("%s.", field.Name)) {
			reflectutils.Set(&setting, strings.TrimPrefix(fieldWithPrefix, fmt.Sprintf("%s.", field.Name)), ctx.R.Form.Get(fieldWithPrefix))
		}
	}
	return reflectutils.Set(obj, field.Name, setting)
}

func (b *Builder) EditingComponentFunc(obj interface{}, field *presets.FieldContext, ctx *web.EventContext) h.HTMLComponent {
	var (
		msgr        = i18n.MustGetModuleMessages(ctx.R, I18nSeoKey, Messages_en_US).(*Messages)
		fieldPrefix string
		setting     Setting
		db          = b.getDBFromContext(ctx.R.Context())
		locale, _   = l10n.IsLocalizableFromCtx(ctx.R.Context())
	)

	seo := b.GetSEO(obj)
	if seo == nil {
		return h.Div()
	}

	value := reflect.Indirect(reflect.ValueOf(obj))
	for i := 0; i < value.NumField(); i++ {
		if s, ok := value.Field(i).Interface().(Setting); ok {
			setting = s
			fieldPrefix = value.Type().Field(i).Name
		}
	}
	if !setting.EnabledCustomize && setting.IsEmpty() {
		modelSetting := &QorSEOSetting{}
		db.Where("name = ? AND locale_code = ?", seo.name, locale).First(modelSetting)
		setting = modelSetting.Setting
	}

	hideActions := false
	if ctx.R.FormValue("hideActionsIconForSEOForm") == "true" {
		hideActions = true
	}
	openCustomizePanel := 1
	if setting.EnabledCustomize {
		openCustomizePanel = 0
	}

	return web.Scope(
		h.Div(
			h.Label(msgr.Seo).Class("v-label theme--light"),
			VExpansionPanels(
				VExpansionPanel(
					VExpansionPanelHeader(
						h.HTMLComponents{
							VSwitch().
								Label(msgr.UseDefaults).Attr("ref", "switchComp").
								Bind("value", "!locals.enabledCustomize").
								Bind("input-value", "!locals.enabledCustomize").
								FieldName(fmt.Sprintf("%s.%s", fieldPrefix, "enabledCustomize")),
						}).
						Attr("style", "padding: 0px 24px;").HideActions(hideActions).
						Attr("@click", "locals.enabledCustomize=!locals.enabledCustomize;$refs.switchComp.$emit('change', locals.enabledCustomize)"),
					VExpansionPanelContent(
						VCardText(
							b.vseo(fieldPrefix, seo, &setting, ctx.R),
						),
					).Eager(true),
				),
			).Flat(true).Attr("v-model", "locals.openCustomizePanel"),
		).Class("pb-4"),
	).Init(fmt.Sprintf(`{enabledCustomize: %t, openCustomizePanel: %d}`, setting.EnabledCustomize, openCustomizePanel)).
		VSlot("{ locals }")
}

func (b *Builder) pageFunc(ctx *web.EventContext) (_ web.PageResponse, err error) {
	var (
		msgr      = i18n.MustGetModuleMessages(ctx.R, I18nSeoKey, Messages_en_US).(*Messages)
		db        = b.getDBFromContext(ctx.R.Context())
		locale, _ = l10n.IsLocalizableFromCtx(ctx.R.Context())
	)

	var seoComponents h.HTMLComponents
	for _, seo := range b.registeredSEO {
		modelSetting := &QorSEOSetting{}
		err := db.Where("name = ? AND locale_code = ?", seo.name, locale).First(modelSetting).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			modelSetting.Name = seo.name
			modelSetting.SetLocale(locale)
			if err := db.Save(modelSetting).Error; err != nil {
				panic(err)
			}
		}

		var variablesComps h.HTMLComponents
		if seo.settingVariables != nil {
			variables := reflect.Indirect(reflect.New(reflect.Indirect(reflect.ValueOf(seo.settingVariables)).Type()))
			variableValues := modelSetting.Variables
			for i := 0; i < variables.NumField(); i++ {
				fieldName := variables.Type().Field(i).Name
				if variableValues[fieldName] != "" {
					fmt.Println(fieldName, variableValues[fieldName])
					variables.Field(i).Set(reflect.ValueOf(variableValues[fieldName]))
				}
			}

			if variables.Type().NumField() > 0 {
				variablesComps = append(variablesComps, h.H3(msgr.Variable).Style("margin-top:15px;font-weight: 500"))
			}

			for i := 0; i < variables.Type().NumField(); i++ {
				field := variables.Type().Field(i)
				variablesComps = append(variablesComps, VTextField().FieldName(fmt.Sprintf("%s.Variables.%s", seo.name, field.Name)).Label(i18n.PT(ctx.R, presets.ModelsI18nModuleKey, "Seo Variable", field.Name)).Value(variables.Field(i).String()))
			}
		}

		var (
			label       string
			setting     = modelSetting.Setting
			loadingName = strings.ReplaceAll(strings.ToLower(seo.name), " ", "_")
		)

		if seo.name == b.globalName {
			label = msgr.GlobalName
		} else {
			label = i18n.PT(ctx.R, presets.ModelsI18nModuleKey, "Seo", seo.name)
		}

		comp := VExpansionPanel(
			VExpansionPanelHeader(h.H4(label).Style("font-weight: 500;")),
			VExpansionPanelContent(
				VCardText(
					variablesComps,
					b.vseo(modelSetting.Name, seo, &setting, ctx.R),
				),
				VCardActions(
					VSpacer(),
					VBtn(msgr.Save).Bind("loading", fmt.Sprintf("vars.%s", loadingName)).Color("primary").Large(true).
						Attr("@click", web.Plaid().
							EventFunc(saveEvent).
							Query("name", seo.name).
							Query("loadingName", loadingName).
							BeforeScript(fmt.Sprintf("vars.%s = true;", loadingName)).Go()),
				).Attr(web.InitContextVars, fmt.Sprintf(`{%s: false}`, loadingName)),
			),
		)
		seoComponents = append(seoComponents, comp)
	}
	return web.PageResponse{
		PageTitle: msgr.PageTitle,
		Body: h.If(editIsAllowed(ctx.R) == nil, VContainer(
			VSnackbar(h.Text(msgr.SavedSuccessfully)).
				Attr("v-model", "vars.seoSnackbarShow").
				Top(true).
				Color("primary").
				Timeout(2000),
			VRow(
				VCol(
					VContainer(
						h.H3(msgr.PageMetadataTitle).Attr("style", "font-weight: 500"),
						h.P().Text(msgr.PageMetadataDescription)),
				).Cols(3),
				VCol(
					VExpansionPanels(
						seoComponents,
					).Focusable(true),
				).Cols(9),
			),
		).Attr("style", "background-color: #f5f5f5;max-width:100%").Attr(web.InitContextVars, `{seoSnackbarShow: false}`)),
	}, nil
}

func (b *Builder) vseo(fieldPrefix string, seo *SEO, setting *Setting, req *http.Request) h.HTMLComponent {
	var (
		seos []*SEO
		msgr = i18n.MustGetModuleMessages(req, I18nSeoKey, Messages_en_US).(*Messages)
		db   = b.getDBFromContext(req.Context())
	)
	if seo.name == b.globalName {
		seos = append(seos, seo)
	} else {
		seos = append(seos, b.GetSEO(b.globalName), seo)
	}

	var (
		variablesEle []h.HTMLComponent
		variables    []string
	)

	for _, seo := range seos {
		if seo.settingVariables != nil {
			value := reflect.Indirect(reflect.ValueOf(seo.settingVariables)).Type()
			for i := 0; i < value.NumField(); i++ {
				fieldName := value.Field(i).Name
				variables = append(variables, fieldName)
			}
		}

		for key := range seo.contextVariables {
			if !strings.Contains(key, ":") {
				variables = append(variables, key)
			}
		}
	}

	var varComps []h.HTMLComponent
	for _, variable := range variables {
		varComps = append(varComps,
			VChip(
				VIcon("add_box").Class("mr-2"),
				h.Text(i18n.PT(req, presets.ModelsI18nModuleKey, "Seo Variable", variable)),
			).Attr("@click", fmt.Sprintf("$refs.seo.addTags('%s')", variable)).Label(true).Outlined(true),
		)
	}
	variablesEle = append(variablesEle, VChipGroup(varComps...).Column(true).Class("ma-4"))

	image := &setting.OpenGraphImageFromMediaLibrary
	if image.ID.String() == "0" {
		image.ID = json.Number("")
	}
	refPrefix := strings.ReplaceAll(strings.ToLower(fieldPrefix), " ", "_")
	return VSeo(
		h.H4(msgr.Basic).Style("margin-top:15px;font-weight: 500"),
		VRow(
			variablesEle...,
		),
		VCard(
			VCardText(
				VTextField().Attr("counter", true).FieldName(fmt.Sprintf("%s.%s", fieldPrefix, "Title")).Label(msgr.Title).Value(setting.Title).Attr("@focus", fmt.Sprintf("$refs.seo.tagInputsFocus($refs.%s)", fmt.Sprintf("%s_title", refPrefix))).Attr("ref", fmt.Sprintf("%s_title", refPrefix)),
				VTextField().Attr("counter", true).FieldName(fmt.Sprintf("%s.%s", fieldPrefix, "Description")).Label(msgr.Description).Value(setting.Description).Attr("@focus", fmt.Sprintf("$refs.seo.tagInputsFocus($refs.%s)", fmt.Sprintf("%s_description", refPrefix))).Attr("ref", fmt.Sprintf("%s_description", refPrefix)),
				VTextarea().Attr("counter", true).Rows(2).AutoGrow(true).FieldName(fmt.Sprintf("%s.%s", fieldPrefix, "Keywords")).Label(msgr.Keywords).Value(setting.Keywords).Attr("@focus", fmt.Sprintf("$refs.seo.tagInputsFocus($refs.%s)", fmt.Sprintf("%s_keywords", refPrefix))).Attr("ref", fmt.Sprintf("%s_keywords", refPrefix)),
			),
		).Outlined(true).Flat(true),

		h.H4(msgr.OpenGraphInformation).Style("margin-top:15px;margin-bottom:15px;font-weight: 500"),
		VCard(
			VCardText(
				VRow(
					VCol(VTextField().FieldName(fmt.Sprintf("%s.%s", fieldPrefix, "OpenGraphTitle")).Label(msgr.OpenGraphTitle).Value(setting.OpenGraphTitle).Attr("@focus", fmt.Sprintf("$refs.seo.tagInputsFocus($refs.%s)", fmt.Sprintf("%s_og_title", refPrefix))).Attr("ref", fmt.Sprintf("%s_og_title", refPrefix))).Cols(6),
					VCol(VTextField().FieldName(fmt.Sprintf("%s.%s", fieldPrefix, "OpenGraphDescription")).Label(msgr.OpenGraphDescription).Value(setting.OpenGraphDescription).Attr("@focus", fmt.Sprintf("$refs.seo.tagInputsFocus($refs.%s)", fmt.Sprintf("%s_og_description", refPrefix))).Attr("ref", fmt.Sprintf("%s_og_description", refPrefix))).Cols(6),
				),
				VRow(
					VCol(VTextField().FieldName(fmt.Sprintf("%s.%s", fieldPrefix, "OpenGraphURL")).Label(msgr.OpenGraphURL).Value(setting.OpenGraphURL).Attr("@focus", fmt.Sprintf("$refs.seo.tagInputsFocus($refs.%s)", fmt.Sprintf("%s_og_url", refPrefix))).Attr("ref", fmt.Sprintf("%s_og_url", refPrefix))).Cols(6),
					VCol(VTextField().FieldName(fmt.Sprintf("%s.%s", fieldPrefix, "OpenGraphType")).Label(msgr.OpenGraphType).Value(setting.OpenGraphType).Attr("@focus", fmt.Sprintf("$refs.seo.tagInputsFocus($refs.%s)", fmt.Sprintf("%s_og_type", refPrefix))).Attr("ref", fmt.Sprintf("%s_og_type", refPrefix))).Cols(6),
				),
				VRow(
					VCol(VTextField().FieldName(fmt.Sprintf("%s.%s", fieldPrefix, "OpenGraphImageURL")).Label(msgr.OpenGraphImageURL).Value(setting.OpenGraphImageURL).Attr("@focus", fmt.Sprintf("$refs.seo.tagInputsFocus($refs.%s)", fmt.Sprintf("%s_og_imageurl", refPrefix))).Attr("ref", fmt.Sprintf("%s_og_imageurl", refPrefix))).Cols(12),
				),
				VRow(
					VCol(views.QMediaBox(db).Label(msgr.OpenGraphImage).
						FieldName(fmt.Sprintf("%s.%s", fieldPrefix, "OpenGraphImageFromMediaLibrary")).
						Value(image).
						Config(&media_library.MediaBoxConfig{
							AllowType: "image",
							Sizes: map[string]*media.Size{
								"og": {
									Width:  1200,
									Height: 630,
								},
								"twitter-large": {
									Width:  1200,
									Height: 600,
								},
								"twitter-small": {
									Width:  630,
									Height: 630,
								},
							},
						})).Cols(12)),
				VRow(
					VCol(VTextarea().FieldName(fmt.Sprintf("%s.%s", fieldPrefix, "OpenGraphMetadataString")).Label(msgr.OpenGraphMetadata).Value(GetOpenGraphMetadataString(setting.OpenGraphMetadata)).Attr("@focus", fmt.Sprintf("$refs.seo.tagInputsFocus($refs.%s)", fmt.Sprintf("%s_og_metadata", refPrefix))).Attr("ref", fmt.Sprintf("%s_og_metadata", refPrefix))).Cols(12),
				),
			),
		).Outlined(true).Flat(true),
	).Attr("ref", "seo")
}

func (b *Builder) save(ctx *web.EventContext) (r web.EventResponse, err error) {
	var (
		db        = b.getDBFromContext(ctx.R.Context())
		name      = ctx.R.FormValue("name")
		setting   = &QorSEOSetting{}
		locale, _ = l10n.IsLocalizableFromCtx(ctx.R.Context())
	)

	if err = db.Where("name = ? AND locale_code = ?", name, locale).First(setting).Error; err != nil {
		return
	}

	var (
		variables   = map[string]string{}
		settingVals = map[string]interface{}{}
		mediaBox    = media_library.MediaBox{}
	)

	for fieldWithPrefix := range ctx.R.Form {
		if !strings.HasPrefix(fieldWithPrefix, name) {
			continue
		}
		field := strings.Replace(fieldWithPrefix, fmt.Sprintf("%s.", name), "", -1)
		// make sure OpenGraphImageFromMediaLibrary.Description set after OpenGraphImageFromMediaLibrary.Values
		if field == "OpenGraphImageFromMediaLibrary.Values" {
			err = mediaBox.Scan(ctx.R.FormValue(fieldWithPrefix))
			if err != nil {
				return
			}
			break
		}
	}

	for fieldWithPrefix := range ctx.R.Form {
		if !strings.HasPrefix(fieldWithPrefix, name) {
			continue
		}
		field := strings.Replace(fieldWithPrefix, fmt.Sprintf("%s.", name), "", -1)
		if strings.HasPrefix(field, "OpenGraphImageFromMediaLibrary") {
			if field == "OpenGraphImageFromMediaLibrary.Description" {
				mediaBox.Description = ctx.R.FormValue(fieldWithPrefix)
				if err != nil {
					return
				}
				settingVals["OpenGraphImageFromMediaLibrary"] = mediaBox
			}
			continue
		}
		if strings.HasPrefix(field, "Variables") {
			key := strings.Replace(field, "Variables.", "", -1)
			variables[key] = ctx.R.FormValue(fieldWithPrefix)
		} else {
			settingVals[field] = ctx.R.Form.Get(fieldWithPrefix)
		}
	}
	s := setting.Setting
	for k, v := range settingVals {
		if k == "OpenGraphMetadataString" {
			metadata := GetOpenGraphMetadata(v.(string))
			err = reflectutils.Set(&s, "OpenGraphMetadata", metadata)
			if err != nil {
				return
			}
			continue
		}
		err = reflectutils.Set(&s, k, v)
		if err != nil {
			return
		}
	}

	setting.Setting = s
	setting.Variables = variables
	setting.SetLocale(locale)
	if err = db.Save(setting).Error; err != nil {
		return
	}
	r.VarsScript = fmt.Sprintf(`vars.seoSnackbarShow = true;vars.%s = false;`, ctx.R.FormValue("loadingName"))
	if b.afterSave != nil {
		if err = b.afterSave(ctx.R.Context(), name, locale); err != nil {
			return
		}
	}
	return
}
