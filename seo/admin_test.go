package seo

import (
	"bytes"
	"errors"
	"github.com/qor5/admin/l10n"
	"github.com/qor5/admin/presets/gorm2op"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	l10n_view "github.com/qor5/admin/l10n/views"
	"github.com/qor5/admin/presets"
	"gorm.io/gorm"
)

func TestAdmin(t *testing.T) {
	var (
		admin  = presets.New().URIPrefix("/admin").DataOperator(gorm2op.DataOperator(dbForTest))
		server = httptest.NewServer(admin)
	)

	builder := NewBuilder(dbForTest, WithLocales("en"))
	builder.RegisterMultipleSEO("Product Detail", "Product")
	l10nBuilder := l10n.New().RegisterLocales("en", "en", "English")
	builder.Configure(admin)
	l10n_view.Configure(admin, dbForTest, l10nBuilder, nil)
	if req, err := http.Get(server.URL + "/admin/qor-seo-settings?__execute_event__=__reload__&locale=en"); err == nil {
		if req.StatusCode != 200 {
			t.Errorf("Setting page should be exist, status code is %v", req.StatusCode)
		}

		var seoSetting []*QorSEOSetting
		dbForTest.Find(&seoSetting, "name in (?)", []string{"Product Detail", "Product", defaultGlobalSEOName})

		if len(seoSetting) != 3 {
			t.Errorf("SEO Setting should be created successfully")
		}
	} else {
		t.Errorf(err.Error())
	}

	// save SEO setting
	var (
		title       = "title test"
		description = "description test"
		keyword     = "keyword test"
	)

	var form = &bytes.Buffer{}
	mwriter := multipart.NewWriter(form)
	mwriter.WriteField("Product.Title", title)
	mwriter.WriteField("Product.Description", description)
	mwriter.WriteField("Product.Keywords", keyword)
	mwriter.Close()

	req, err := http.DefaultClient.Post(server.URL+"/admin/qor-seo-settings?__execute_event__=seo_save_collection&name=Product&locale=en", mwriter.FormDataContentType(), form)
	if err != nil {
		t.Fatal(err)
	}

	if req.StatusCode != 200 {
		t.Errorf("Save should be processed successfully, status code is %v", req.StatusCode)
	}

	seoSetting := &QorSEOSetting{}
	err = dbForTest.First(seoSetting, "name = ?", "Product").Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("SEO Setting should be created successfully")
	}

	setting := seoSetting.Setting
	if setting.Title != title || setting.Description != description || setting.Keywords != keyword {
		t.Errorf("SEOSetting should be Save correctly, its value %#v", seoSetting)
	}
}
