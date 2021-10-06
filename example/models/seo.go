package models

import (
	"github.com/goplaid/x/presets"
	"github.com/qor/qor5/seo"
	"gorm.io/gorm"
)

var SeoCollection *seo.Collection

type GlobalVaribles struct {
	SiteName string
}

func ConfigureSeo(b *presets.Builder, db *gorm.DB) {
	SeoCollection = seo.New("Site SEO")
	SeoCollection.RegisterGlobalVariables(&GlobalVaribles{})
	SeoCollection.RegisterSettingModel(&seo.QorSEOSetting{})

	SeoCollection.RegisterSEO(&seo.SEO{
		Name: "Not Found",
	})

	SeoCollection.RegisterSEO(&seo.SEO{
		Name: "Internal Server Error",
	})

	SeoCollection.RegisterSEO(&seo.SEO{
		Name:      "Post",
		Variables: []string{"Title"},
		Context: func(objects ...interface{}) map[string]string {
			context := make(map[string]string)
			if len(objects) > 0 {
				if article, ok := objects[0].(Post); ok {
					context["Title"] = article.Title
				}
			}
			return context
		},
	})

	SeoCollection.Configure(b, db)
}
