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

func (b *Builder) Configure(pb *presets.Builder, db *gorm.DB, l10nBuilder ...*l10n.Builder) (pm *presets.ModelBuilder) {
	if err := db.AutoMigrate(&QorSEOSetting{}); err != nil {
		panic(err)
	}

	if GlobalDB == nil {
		GlobalDB = db
	}

	var locales []string
	if len(l10nBuilder) > 0 {
		GlobalL10n = l10nBuilder[0]
		locales = GlobalL10n.GetSupportLocaleCodes()
	}

	// insert records into database
	b.dummyNode.Migrate(locales)

	pb.GetWebBuilder().RegisterEventFunc(saveEvent, b.save)

	pm = pb.Model(&QorSEOSetting{}).PrimaryField("Name").Label("SEO")
	// Configure Listing Page
	{

		pml := pm.Listing("Name")
		// disable new btn globally, no one can add new SEO record after server start up.
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
			cond := presets.SQLCondition{
				Query: "locale_code = ?",
				Args:  []interface{}{locale},
			}
			params.SQLConditions = append(params.SQLConditions, &cond)
			r, totalCount, err = oldSearcher(model, params, ctx)
			b.SortSEOs(r.([]*QorSEOSetting))
			return
		})
	}
	// Configure Editing
	pme := pm.Editing("Setting")
	pme.Field("Setting").ComponentFunc(func(obj interface{}, field *presets.FieldContext, ctx *web.EventContext) h.HTMLComponent {
		seoSetting := obj.(*QorSEOSetting)
		msgr := i18n.MustGetModuleMessages(ctx.R, I18nSeoKey, Messages_en_US).(*Messages)
		settingVars := b.GetSEO(seoSetting.Name).settingVars
		var variablesComps h.HTMLComponents
		if len(settingVars) > 0 {
			variablesComps = append(variablesComps, h.H3(msgr.Variable).Style("margin-top:15px;font-weight: 500"))
			for varName, _ := range settingVars {
				fieldComp := VTextField().
					FieldName(fmt.Sprintf("%s.Variables.%s", seoSetting.Name, field.Name)).
					Label(i18n.PT(ctx.R, presets.ModelsI18nModuleKey, "Seo Variable", field.Name)).
					Value(varName)
				variablesComps = append(variablesComps, fieldComp)
			}
		}
		return VCardText(
			variablesComps,
			b.vseo("Setting", b.GetSEO(seoSetting.Name), &seoSetting.Setting, ctx.R),
		)
	})
	pb.FieldDefaults(presets.WRITE).
		FieldType(Setting{}).
		ComponentFunc(b.EditingComponentFunc).
		SetterFunc(EditSetterFunc)

	pb.I18n().
		RegisterForModule(language.English, I18nSeoKey, Messages_en_US).
		RegisterForModule(language.SimplifiedChinese, I18nSeoKey, Messages_zh_CN)

	pb.ExtraAsset("/vue-SEO.js", "text/javascript", SeoJSComponentsPack())
	permVerifier = perm.NewVerifier("SEO", pb.GetPermission())
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
	var seo *SEO
	if qs, ok := obj.(*QorSEOSetting); ok {
		seo = b.GetSEO(qs.Name)
	} else {
		seo = b.GetSEO(obj)
	}
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

func (b *Builder) vseo(fieldPrefix string, seo *SEO, setting *Setting, req *http.Request) h.HTMLComponent {
	var (
		msgr = i18n.MustGetModuleMessages(req, I18nSeoKey, Messages_en_US).(*Messages)
		db   = b.getDBFromContext(req.Context())
	)

	var varComps []h.HTMLComponent
	for varName := range seo.GetAvailableVars() {
		varComps = append(varComps,
			VChip(
				VIcon("add_box").Class("mr-2"),
				h.Text(i18n.PT(req, presets.ModelsI18nModuleKey, "Seo Variable", varName)),
			).Attr("@click", fmt.Sprintf("$refs.SEO.addTags('%s')", varName)).Label(true).Outlined(true),
		)
	}
	var variablesEle []h.HTMLComponent
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
				VTextField().Attr("counter", true).FieldName(fmt.Sprintf("%s.%s", fieldPrefix, "Title")).Label(msgr.Title).Value(setting.Title).Attr("@focus", fmt.Sprintf("$refs.SEO.tagInputsFocus($refs.%s)", fmt.Sprintf("%s_title", refPrefix))).Attr("ref", fmt.Sprintf("%s_title", refPrefix)),
				VTextField().Attr("counter", true).FieldName(fmt.Sprintf("%s.%s", fieldPrefix, "Description")).Label(msgr.Description).Value(setting.Description).Attr("@focus", fmt.Sprintf("$refs.SEO.tagInputsFocus($refs.%s)", fmt.Sprintf("%s_description", refPrefix))).Attr("ref", fmt.Sprintf("%s_description", refPrefix)),
				VTextarea().Attr("counter", true).Rows(2).AutoGrow(true).FieldName(fmt.Sprintf("%s.%s", fieldPrefix, "Keywords")).Label(msgr.Keywords).Value(setting.Keywords).Attr("@focus", fmt.Sprintf("$refs.SEO.tagInputsFocus($refs.%s)", fmt.Sprintf("%s_keywords", refPrefix))).Attr("ref", fmt.Sprintf("%s_keywords", refPrefix)),
			),
		).Outlined(true).Flat(true),

		h.H4(msgr.OpenGraphInformation).Style("margin-top:15px;margin-bottom:15px;font-weight: 500"),
		VCard(
			VCardText(
				VRow(
					VCol(VTextField().FieldName(fmt.Sprintf("%s.%s", fieldPrefix, "OpenGraphTitle")).Label(msgr.OpenGraphTitle).Value(setting.OpenGraphTitle).Attr("@focus", fmt.Sprintf("$refs.SEO.tagInputsFocus($refs.%s)", fmt.Sprintf("%s_og_title", refPrefix))).Attr("ref", fmt.Sprintf("%s_og_title", refPrefix))).Cols(6),
					VCol(VTextField().FieldName(fmt.Sprintf("%s.%s", fieldPrefix, "OpenGraphDescription")).Label(msgr.OpenGraphDescription).Value(setting.OpenGraphDescription).Attr("@focus", fmt.Sprintf("$refs.SEO.tagInputsFocus($refs.%s)", fmt.Sprintf("%s_og_description", refPrefix))).Attr("ref", fmt.Sprintf("%s_og_description", refPrefix))).Cols(6),
				),
				VRow(
					VCol(VTextField().FieldName(fmt.Sprintf("%s.%s", fieldPrefix, "OpenGraphURL")).Label(msgr.OpenGraphURL).Value(setting.OpenGraphURL).Attr("@focus", fmt.Sprintf("$refs.SEO.tagInputsFocus($refs.%s)", fmt.Sprintf("%s_og_url", refPrefix))).Attr("ref", fmt.Sprintf("%s_og_url", refPrefix))).Cols(6),
					VCol(VTextField().FieldName(fmt.Sprintf("%s.%s", fieldPrefix, "OpenGraphType")).Label(msgr.OpenGraphType).Value(setting.OpenGraphType).Attr("@focus", fmt.Sprintf("$refs.SEO.tagInputsFocus($refs.%s)", fmt.Sprintf("%s_og_type", refPrefix))).Attr("ref", fmt.Sprintf("%s_og_type", refPrefix))).Cols(6),
				),
				VRow(
					VCol(VTextField().FieldName(fmt.Sprintf("%s.%s", fieldPrefix, "OpenGraphImageURL")).Label(msgr.OpenGraphImageURL).Value(setting.OpenGraphImageURL).Attr("@focus", fmt.Sprintf("$refs.SEO.tagInputsFocus($refs.%s)", fmt.Sprintf("%s_og_imageurl", refPrefix))).Attr("ref", fmt.Sprintf("%s_og_imageurl", refPrefix))).Cols(12),
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
					VCol(VTextarea().FieldName(fmt.Sprintf("%s.%s", fieldPrefix, "OpenGraphMetadataString")).Label(msgr.OpenGraphMetadata).Value(GetOpenGraphMetadataString(setting.OpenGraphMetadata)).Attr("@focus", fmt.Sprintf("$refs.SEO.tagInputsFocus($refs.%s)", fmt.Sprintf("%s_og_metadata", refPrefix))).Attr("ref", fmt.Sprintf("%s_og_metadata", refPrefix))).Cols(12),
				),
			),
		).Outlined(true).Flat(true),
	).Attr("ref", "SEO")
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
