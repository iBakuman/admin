package seo

import (
	"context"
	"github.com/theplant/testingutils"
	"net/http"
	"net/url"
	"testing"

	_ "github.com/lib/pq"
)

func TestBuilder_Render(t *testing.T) {
	u, _ := url.Parse("http://dev.qor5.com/product/1")
	defaultRequest := &http.Request{
		Method: "GET",
		URL:    u,
	}

	globalSeoSetting := QorSEOSetting{
		Name: "Global SEO",
		Setting: Setting{
			Title: "global | {{SiteName}}",
		},
		Variables: map[string]string{"SiteName": "Qor5 dev"},
	}

	newBuilderWithGlobalSEO := func() (*Builder, *SEO) {
		builder := NewBuilder()
		globalSEO := builder.RegisterSEO("Global SEO")
		globalSEO.RegisterSettingVariables("SiteName").
			RegisterPropFuncForOG(
				&PropFunc{
					Name: "og:url",
					Func: func(_ interface{}, _ *Setting, req *http.Request) string {
						return req.URL.String()
					},
				},
			)
		return builder, globalSEO
	}
	tests := []struct {
		name      string
		prepareDB func()
		builder   *Builder
		obj       interface{}
		want      string
	}{
		{
			name:      "Render Global SEO with setting variables and default context variables",
			prepareDB: func() { GlobalDB.Save(&globalSeoSetting) },
			builder: func() *Builder {
				builder, _ := newBuilderWithGlobalSEO()
				return builder
			}(),
			obj: "Global SEO",
			want: `
			<title>global | Qor5 dev</title>
			<meta property='og:url' name='og:url' content='http://dev.qor5.com/product/1'>
			`,
		},
		{
			name: "Render SEO setting with global setting variables",
			prepareDB: func() {
				GlobalDB.Save(&globalSeoSetting)
				product := QorSEOSetting{
					Name: "Product",
					Setting: Setting{
						Title: "product | {{SiteName}}",
					},
				}
				GlobalDB.Save(&product)
			},
			builder: func() *Builder {
				builder, seoRoot := newBuilderWithGlobalSEO()
				builder.RegisterSEO("Product").SetParent(seoRoot)
				return builder
			}(),
			obj:  "Product",
			want: `<title>product | Qor5 dev</title>`,
		},

		{
			name: "Render SEO setting with setting and opengraph prop",
			prepareDB: func() {
				GlobalDB.Save(&globalSeoSetting)
				product := QorSEOSetting{
					Name: "Product",
					Setting: Setting{
						Title: "product {{ProductTag}} | {{SiteName}}",
					},
					Variables: map[string]string{"ProductTag": "Men"},
				}
				GlobalDB.Save(&product)
			},
			builder: func() *Builder {
				builder, globalSEO := newBuilderWithGlobalSEO()
				builder.RegisterSEO("Product").
					RegisterSettingVariables("ProductTag ").
					RegisterPropFuncForOG(
						&PropFunc{
							Name: "og:image",
							Func: func(i interface{}, setting *Setting, request *http.Request) string {
								return "http://dev.qor5.com/images/logo.png"
							},
						},
					).SetParent(globalSEO)
				return builder
			}(),
			obj: "Product",
			want: `
			<title>product Men | Qor5 dev</title>
			<meta property='og:image' name='og:image' content='http://dev.qor5.com/images/logo.png'>`,
		},

		{
			name: "Render model setting with global and SEO setting variables",
			prepareDB: func() {
				GlobalDB.Save(&globalSeoSetting)
				product := QorSEOSetting{
					Name:      "Product",
					Variables: map[string]string{"ProductTag": "Men"},
				}
				GlobalDB.Save(&product)
			},
			builder: func() *Builder {
				builder := NewBuilder()
				builder.RegisterSEO(&Product{})
				return builder
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
			name: "Render model setting with default SEO setting",
			prepareDB: func() {
				GlobalDB.Save(&globalSeoSetting)
				product := QorSEOSetting{
					Name: "Product",
					Setting: Setting{
						Title: "product | Qor5 dev",
					},
					Variables: map[string]string{"ProductTag": "Men"},
				}
				GlobalDB.Save(&product)
			},
			builder: func() *Builder {
				builder := NewBuilder()
				builder.RegisterSEO(&Product{})
				return builder
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
			name: "Render model setting with inherit global and SEO setting",
			prepareDB: func() {
				GlobalDB.Save(&globalSeoSetting)
				product := QorSEOSetting{
					Name: "Product",
					Setting: Setting{
						Description: "product description",
					},
					Variables: map[string]string{"ProductTag": "Men"},
				}
				GlobalDB.Save(&product)
			},
			builder: func() *Builder {
				builder := NewBuilder()
				builder.RegisterSEO(&Product{})
				return builder
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
			name: "Render model setting without inherit global and SEO setting",
			prepareDB: func() {
				GlobalDB.Save(&globalSeoSetting)
				product := QorSEOSetting{
					Name: "Product",
					Setting: Setting{
						Description: "product description",
					},
					Variables: map[string]string{"ProductTag": "Men"},
				}
				GlobalDB.Save(&product)
			},
			builder: func() *Builder {
				builder := NewBuilder()
				builder.RegisterSEO(&Product{})
				return builder
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
			if got, _ := tt.builder.Render(tt.obj, defaultRequest).MarshalHTML(context.TODO()); !metaEqual(string(got), tt.want) {
				t.Errorf("Render = %v\nExpected = %v", string(got), tt.want)
			}
		})
	}

}

func TestBuilder_GetSEOPriority(t *testing.T) {
	cases := []struct {
		name     string
		builder  *Builder
		expected map[string]int
	}{
		{
			name: "case 1",
			builder: func() *Builder {
				builder := NewBuilder()
				builder.RegisterSEO("PLP").AppendChildren(
					builder.RegisterSEO("Region"),
					builder.RegisterSEO("City"),
					builder.RegisterSEO("Prefecture"),
				).AppendChildren(
					builder.RegisterMultipleSEO("Post", "Product")...,
				)
				return builder
			}(),
			expected: map[string]int{
				"Global SEO": 1,
				"PLP":        2,
				"Post":       3,
				"Product":    3,
				"Region":     3,
				"City":       3,
				"Prefecture": 3,
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for seoName, priority := range c.expected {
				actualPriority := c.builder.GetSEOPriority(seoName)
				if actualPriority != priority {
					t.Errorf("GetPriorities = %v, want %v", actualPriority, priority)
				}
			}
		})
	}
}

func TestBuilder_RemoveSEO(t *testing.T) {
	cases := []struct {
		name     string
		builder  *Builder
		expected *Builder
	}{{
		name: "test remove SEO",
		builder: func() *Builder {
			builder := NewBuilder()
			builder.RegisterSEO("Parent1").AppendChildren(
				builder.RegisterSEO("Son1"),
				builder.RegisterSEO("Son2"),
			)
			builder.RemoveSEO("Parent1")
			return builder
		}(),
		expected: func() *Builder {
			builder := NewBuilder()
			builder.RegisterMultipleSEO("Son1", "Son2")
			return builder
		}(),
	}}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actual := c.builder
			expected := c.expected
			if len(actual.registeredSEO) != len(expected.registeredSEO) {
				t.Errorf("The length is not equal")
			}
			for _, desired := range expected.registeredSEO {
				if seo := actual.GetSEO(desired.name); seo == nil {
					t.Errorf("not found SEO %v in actual", desired.name)
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

func TestBuilder_SortSEOs(t *testing.T) {

	cases := []struct {
		name     string
		builder  *Builder
		data     []*QorSEOSetting
		expected []*QorSEOSetting
	}{
		{
			name: "test_sort_seo",
			builder: func() *Builder {
				builder := NewBuilder()
				builder.RegisterSEO("PLP").AppendChildren(
					builder.RegisterSEO("Region"),
					builder.RegisterSEO("City"),
					builder.RegisterSEO("Prefecture"),
				)
				builder.RegisterMultipleSEO("Post", "Product")
				return builder
			}(),
			data: []*QorSEOSetting{
				{Name: "Post"},
				{Name: "Region"},
				{Name: "PLP"},
				{Name: "Global SEO"},
				{Name: "City"},
				{Name: "Prefecture"},
				{Name: "Product"}},
			expected: []*QorSEOSetting{
				{Name: "Global SEO"},
				{Name: "PLP"},
				{Name: "Region"},
				{Name: "City"},
				{Name: "Prefecture"},
				{Name: "Post"},
				{Name: "Product"}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.builder.SortSEOs(c.data)
			r := testingutils.PrettyJsonDiff(c.expected, c.data)
			if r != "" {
				t.Errorf(r)
			}
		})
	}
}
