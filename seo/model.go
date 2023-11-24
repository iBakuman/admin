package seo

import (
	"bytes"
	"database/sql/driver"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/qor5/admin/l10n"
	"github.com/qor5/admin/media/media_library"
	h "github.com/theplant/htmlgo"
)

// QorSEOSetting default seo model
type QorSEOSetting struct {
	Name      string `gorm:"primary_key"`
	Setting   Setting
	Variables Variables `sql:"type:text"`

	Path      string
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time `gorm:"index"`
	l10n.Locale
}

func (s *QorSEOSetting) PrimarySlug() string {
	return fmt.Sprintf("%v_%v", s.Name, s.LocaleCode)
}

func (s *QorSEOSetting) PrimaryColumnValuesBySlug(slug string) map[string]string {
	segs := strings.Split(slug, "_")
	if len(segs) != 2 {
		panic("wrong slug")
	}

	return map[string]string{
		"name":        segs[0],
		"locale_code": segs[1],
	}
}

// Setting defined meta's attributes
type Setting struct {
	Title                          string                 `gorm:"size:4294967295" json:",omitempty"`
	Description                    string                 `json:",omitempty"`
	Keywords                       string                 `json:",omitempty"`
	OpenGraphTitle                 string                 `json:",omitempty"`
	OpenGraphDescription           string                 `json:",omitempty"`
	OpenGraphURL                   string                 `json:",omitempty"`
	OpenGraphType                  string                 `json:",omitempty"`
	OpenGraphImageURL              string                 `json:",omitempty"`
	OpenGraphImageFromMediaLibrary media_library.MediaBox `json:",omitempty"`
	OpenGraphMetadata              []OpenGraphMetadata    `json:",omitempty"`
	EnabledCustomize               bool                   `json:",omitempty"`
}

// OpenGraphMetadata open graph meta data
type OpenGraphMetadata struct {
	Property string
	Content  string
}

// Scan scan value from database into struct
func (setting *Setting) Scan(value interface{}) error {
	if byteList, ok := value.([]byte); ok {
		json.Unmarshal(byteList, setting)
	} else if str, ok := value.(string); ok {
		json.Unmarshal([]byte(str), setting)
	} else if strList, ok := value.([]string); ok {
		for _, str := range strList {
			json.Unmarshal([]byte(str), setting)
		}
	}
	return nil
}

// Value get value from struct, and save into database
func (setting *Setting) Value() (driver.Value, error) {
	result, err := json.Marshal(setting)
	return string(result), err
}

func (setting *Setting) IsEmpty() bool {
	return setting.Title == "" && setting.Description == "" && setting.Keywords == "" &&
		setting.OpenGraphTitle == "" && setting.OpenGraphDescription == "" &&
		setting.OpenGraphURL == "" && setting.OpenGraphType == "" && setting.OpenGraphImageURL == "" &&
		setting.OpenGraphImageFromMediaLibrary.Url == "" && len(setting.OpenGraphMetadata) == 0
}

type Variables map[string]string

// Scan scan value from database into struct
func (setting *Variables) Scan(value interface{}) error {
	if byteList, ok := value.([]byte); ok {
		json.Unmarshal(byteList, setting)
	} else if str, ok := value.(string); ok {
		json.Unmarshal([]byte(str), setting)
	} else if strList, ok := value.([]string); ok {
		for _, str := range strList {
			json.Unmarshal([]byte(str), setting)
		}
	}
	return nil
}

// Value get value from struct, and save into database
func (setting *Variables) Value() (driver.Value, error) {
	result, err := json.Marshal(setting)
	return string(result), err
}

func (setting *Setting) HTMLComponent(tags map[string]string) h.HTMLComponent {
	openGraphData := map[string]string{
		"og:title":       setting.OpenGraphTitle,
		"og:description": setting.OpenGraphDescription,
		"og:url":         setting.OpenGraphURL,
		"og:type":        setting.OpenGraphType,
		"og:image":       setting.OpenGraphImageURL,
	}

	for _, metaVal := range setting.OpenGraphMetadata {
		openGraphData[metaVal.Property] = metaVal.Content
	}

	for _, key := range []string{"og:title", "og:description", "og:url", "og:type", "og:image"} {
		if v := openGraphData[key]; v == "" {
			if v, ok := tags[key]; ok {
				openGraphData[key] = v
			}
		}
	}

	if openGraphData["og:type"] == "" {
		openGraphData["og:type"] = "website"
	}

	for key, value := range tags {
		if _, ok := openGraphData[key]; !ok {
			openGraphData[key] = value
		}
	}

	var openGraphDataComponents h.HTMLComponents
	for key, value := range openGraphData {
		openGraphDataComponents = append(openGraphDataComponents, h.Meta().Attr("property", key).Attr("name", key).Attr("content", value))
	}

	return h.HTMLComponents{
		h.Title(setting.Title),
		h.Meta().Attr("name", "description").Attr("content", setting.Description),
		h.Meta().Attr("name", "keywords").Attr("content", setting.Keywords),
		openGraphDataComponents,
	}
}

func GetOpenGraphMetadata(in string) (metadata []OpenGraphMetadata) {
	r := csv.NewReader(strings.NewReader(in))
	records, err := r.ReadAll()
	if err != nil {
		return
	}
	for _, row := range records {
		if len(row) != 2 {
			continue
		}
		metadata = append(metadata, OpenGraphMetadata{
			Property: row[0],
			Content:  row[1],
		})
	}
	return
}

func GetOpenGraphMetadataString(metadata []OpenGraphMetadata) string {
	var records [][]string
	for _, m := range metadata {
		records = append(records, []string{m.Property, m.Content})
	}
	buf := new(bytes.Buffer)
	w := csv.NewWriter(buf)
	w.WriteAll(records)
	return buf.String()
}
