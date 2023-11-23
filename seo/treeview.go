package seo

import (
	. "github.com/qor5/ui/vuetify"
	"github.com/qor5/web"
)

type Item struct {
	ID       int     `json:"id"`
	Name     string  `json:"name"`
	Children []*Item `json:"children"`
}

func pageFunc1(ctx *web.EventContext) (web.PageResponse, error) {
	root := []*Item{
		{
			ID:   1,
			Name: "Global",
			Children: []*Item{
				{
					ID:   2,
					Name: "Post",
				},
				{
					ID:   3,
					Name: "PLP",
					Children: []*Item{
						{
							ID:   4,
							Name: "Region",
						},
						{
							ID:   5,
							Name: "Prefecture",
						},
						{
							ID:   6,
							Name: "City",
						},
					},
				},
			},
		},
	}

	treeview := VTreeview().OpenAll(true).
		Items(root).
		SelectionType("independent").
		Dense(true).
		Hoverable(true).
		Children(
		// h.Template().Attr("v-slot:append","{item}").Children(
		// 	)
		)
	return web.PageResponse{
		PageTitle: "Test",
		Body:      treeview,
	}, nil
}
