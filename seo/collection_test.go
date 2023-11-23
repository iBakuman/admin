package seo

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	_ "github.com/lib/pq"
)

func TestRender(t *testing.T) {
	u, _ := url.Parse("http://dev.qor5.com/product/1")
	defaultRequest := &http.Request{
		Method: "GET",
		URL:    u,
	}

	globalSeoSetting := TestQorSEOSetting{
		QorSEOSetting: QorSEOSetting{
			Name: GlobalSEOName,
			Setting: Setting{
				Title: "global | {{SiteName}}",
			},
			Variables: map[string]string{"SiteName": "Qor5 dev"},
		},
	}

	tests := []struct {
		name       string
		prepareDB  func()
		collection *Collection
		obj        interface{}
		want       string
	}{
		{
			name:       "Render Golabl SEO with setting variables and default context variables",
			prepareDB:  func() { GlobalDB.Save(&globalSeoSetting) },
			collection: NewCollection().SetSettingModel(&TestQorSEOSetting{}),
			obj:        GlobalSEOName,
			want: `
			<title>global | Qor5 dev</title>
			<meta property='og:url' name='og:url' content='http://dev.qor5.com/product/1'>
			`,
		},

		{
			name: "Render seo setting with global setting variables",
			prepareDB: func() {
				GlobalDB.Save(&globalSeoSetting)
				product := TestQorSEOSetting{
					QorSEOSetting: QorSEOSetting{
						Name: "Product",
						Setting: Setting{
							Title: "product | {{SiteName}}",
						},
					},
				}
				GlobalDB.Save(&product)
			},
			collection: NewCollection().SetSettingModel(&TestQorSEOSetting{}).RegisterSEOByNames("Product"),
			obj:        "Product",
			want:       `<title>product | Qor5 dev</title>`,
		},

		{
			name: "Render seo setting with setting and context variables",
			prepareDB: func() {
				GlobalDB.Save(&globalSeoSetting)
				product := TestQorSEOSetting{
					QorSEOSetting: QorSEOSetting{
						Name: "Product",
						Setting: Setting{
							Title: "product {{ProductTag}} | {{SiteName}}",
						},
						Variables: map[string]string{"ProductTag": "Men"},
					},
				}
				GlobalDB.Save(&product)
			},
			collection: func() *Collection {
				collection := NewCollection().SetSettingModel(&TestQorSEOSetting{})
				collection.RegisterSEO("Product").
					RegisterSettingVaribles(struct{ ProductTag string }{}).
					RegisterContextVariables("og:image", func(_ interface{}, _ *Setting, _ *http.Request) string {
						return "http://dev.qor5.com/images/logo.png"
					})
				return collection
			}(),
			obj: "Product",
			want: `
			<title>product Men | Qor5 dev</title>
			<meta property='og:image' name='og:image' content='http://dev.qor5.com/images/logo.png'>`,
		},

		{
			name: "Render model setting with global and seo setting variables",
			prepareDB: func() {
				GlobalDB.Save(&globalSeoSetting)
				product := TestQorSEOSetting{
					QorSEOSetting: QorSEOSetting{
						Name:      "Product",
						Variables: map[string]string{"ProductTag": "Men"},
					},
				}
				GlobalDB.Save(&product)
			},
			collection: func() *Collection {
				collection := NewCollection().SetSettingModel(&TestQorSEOSetting{})
				collection.RegisterSEO(&Product{})
				return collection
			}(),
			obj: Product{
				Name: "product 1",
				SEO: Setting{
					Title:            "product1 | {{ProductTag}} | {{SiteName}}",
					EnabledCustomize: true,
				},
			},
			want: `<title>product1 | Men | Qor5 dev</title>`,
		},

		{
			name: "Render model setting with default seo setting",
			prepareDB: func() {
				GlobalDB.Save(&globalSeoSetting)
				product := TestQorSEOSetting{
					QorSEOSetting: QorSEOSetting{
						Name: "Product",
						Setting: Setting{
							Title: "product | Qor5 dev",
						},
						Variables: map[string]string{"ProductTag": "Men"},
					},
				}
				GlobalDB.Save(&product)
			},
			collection: func() *Collection {
				collection := NewCollection().SetSettingModel(&TestQorSEOSetting{})
				collection.RegisterSEO(&Product{})
				return collection
			}(),
			obj: Product{
				Name: "product 1",
				SEO: Setting{
					Title:            "product1 | {{ProductTag}} | {{SiteName}}",
					EnabledCustomize: false,
				},
			},
			want: `<title>product | Qor5 dev</title>`,
		},

		{
			name: "Render model setting with inherite gloabl and seo setting",
			prepareDB: func() {
				GlobalDB.Save(&globalSeoSetting)
				product := TestQorSEOSetting{
					QorSEOSetting: QorSEOSetting{
						Name: "Product",
						Setting: Setting{
							Description: "product description",
						},
						Variables: map[string]string{"ProductTag": "Men"},
					},
				}
				GlobalDB.Save(&product)
			},
			collection: func() *Collection {
				collection := NewCollection().SetSettingModel(&TestQorSEOSetting{})
				collection.RegisterSEO(&Product{})
				return collection
			}(),
			obj: Product{
				Name: "product 1",
				SEO: Setting{
					Keywords:         "shoes, {{ProductTag}}",
					EnabledCustomize: true,
				},
			},
			want: `
			<title>global | Qor5 dev</title>
			<meta name='description' content='product description'>
			<meta name='keywords' content='shoes, Men'>
			`,
		},

		{
			name: "Render model setting without inherite gloabl and seo setting",
			prepareDB: func() {
				GlobalDB.Save(&globalSeoSetting)
				product := TestQorSEOSetting{
					QorSEOSetting: QorSEOSetting{
						Name: "Product",
						Setting: Setting{
							Description: "product description",
						},
						Variables: map[string]string{"ProductTag": "Men"},
					},
				}
				GlobalDB.Save(&product)
			},
			collection: func() *Collection {
				collection := NewCollection().SetInherited(false).SetSettingModel(&TestQorSEOSetting{})
				collection.RegisterSEO(&Product{})
				return collection
			}(),
			obj: Product{
				Name: "product 1",
				SEO: Setting{
					Keywords:         "shoes, {{ProductTag}}",
					EnabledCustomize: true,
				},
			},
			want: `
			<title></title>
			<meta name='description'>
			<meta name='keywords' content='shoes, Men'>
			`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetDB()
			tt.prepareDB()
			if got, _ := tt.collection.Render(tt.obj, defaultRequest).MarshalHTML(context.TODO()); !metaEqual(string(got), tt.want) {
				t.Errorf("Render = %v, want %v", string(got), tt.want)
			}
		})
	}

}

func TestCollection_GetSEOPriorities(t *testing.T) {
	cases := []struct {
		name       string
		collection *Collection
		expected   map[string]int
	}{
		{
			name: "case 1",
			collection: func() *Collection {
				collection := NewCollection()
				collection.RegisterSEO("PLP").AddChildren(
					collection.RegisterSEO("Region"),
					collection.RegisterSEO("City"),
					collection.RegisterSEO("Prefecture"),
				)
				collection.RegisterMultipleSEO("Post", "Product")
				return collection
			}(),
			expected: map[string]int{
				"Global SEO": 1,
				"PLP":        2,
				"Post":       2,
				"Product":    2,
				"Region":     3,
				"City":       3,
				"Prefecture": 3,
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actual := c.collection.GetSEOPriorities()
			if len(actual) != len(c.expected) {
				t.Errorf("GetPriorities = %v, want %v", actual, c.expected)
			}
			for seoName, desired := range c.expected {
				if actual[seoName] != desired {
					t.Errorf("The priority of %v is %v, but want %v", seoName, actual[seoName], desired)
				}
			}
		})
	}
}

func TestCollection_RemoveSEO(t *testing.T) {
	cases := []struct {
		name       string
		collection *Collection
		expected   *Collection
	}{{
		name: "test remove seo",
		collection: func() *Collection {
			collection := NewCollection()
			collection.RegisterSEO("Parent1").AddChildren(
				collection.RegisterSEO("Son1"),
				collection.RegisterSEO("Son2"),
			)
			collection.RemoveSEO("Parent1")
			return collection
		}(),
		expected: func() *Collection {
			collection := NewCollection()
			collection.RegisterMultipleSEO("Son1", "Son2")
			return collection
		}(),
	}}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actual := c.collection
			expected := c.expected
			if len(actual.registeredSEO) != len(expected.registeredSEO) {
				t.Errorf("The length is not equal")
			}
			for _, desired := range expected.registeredSEO {
				if seo := actual.GetSEOByName(desired.name); seo == nil {
					t.Errorf("not found seo %v in actual", desired.name)
				} else {
					if seo.parent == nil {
						if desired.parent != nil {
							t.Errorf("actual parent is nil, expected: %s", desired.parent.name)
						}
					} else {
						if seo.parent.name != desired.parent.name {
							t.Errorf("actual parent is %s, expected: %s", seo.parent.name, desired.parent.name)
						}
					}
				}
			}
		})
	}
}

func TestCollection_GetListingOrders(t *testing.T) {

	cases := []struct {
		name       string
		collection *Collection
		expected   *SEO
	}{
		{
			name: "test_get_listing_orders",
			collection: func() *Collection {
				collection := NewCollection()
				collection.RegisterSEO("PLP").AddChildren(
					collection.RegisterSEO("Region"),
					collection.RegisterSEO("City"),
					collection.RegisterSEO("Prefecture"),
				)
				collection.RegisterMultipleSEO("Post", "Product")
				return collection
			}(),
			expected: nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fmt.Println(c.collection.GetListingOrders())
		})
	}
}
