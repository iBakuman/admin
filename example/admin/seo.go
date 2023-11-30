package admin

import (
	"github.com/qor5/admin/l10n"
	"net/http"

	"github.com/qor5/admin/example/models"
	"github.com/qor5/admin/presets"
	"github.com/qor5/admin/seo"
	"gorm.io/gorm"
)

// @snippet_begin(SeoExample)
var seoBuilder *seo.Builder

func ConfigureSeo(b *presets.Builder, db *gorm.DB, l10nBuilder ...*l10n.Builder) *presets.ModelBuilder {
	seoBuilder = seo.NewBuilder()
	seoBuilder.RegisterSEO(&models.Post{}).
		RegisterContextVariables(
			&seo.ContextVar{
				Name: "Title",
				Func: func(object interface{}, _ *seo.Setting, _ *http.Request) string {
					if article, ok := object.(models.Post); ok {
						return article.Title
					}
					return ""
				}}).
		RegisterSettingVariables("Test")
	return seoBuilder.Configure(b, db, l10nBuilder...)
}

// @snippet_end
