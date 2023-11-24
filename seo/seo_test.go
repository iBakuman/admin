package seo

import (
	"testing"
)

func TestSEO_AddChildren(t *testing.T) {
	cases := []struct {
		name     string
		seoRoot  *SEO
		expected [][]string
	}{
		{
			name: "test_seo_add_self_as_child",
			seoRoot: func() *SEO {
				rootSeo := &SEO{name: "Root"}
				node1 := &SEO{name: "Node1"}
				node2 := &SEO{name: "Node2"}
				node3 := &SEO{name: "Node3"}
				rootSeo.AppendChildren(node1, node2)
				node2.AppendChildren(node3)
				// add self as child
				node2.AppendChildren(node2)
				return rootSeo
			}(),
			expected: [][]string{{"Root"}, {"Node1", "Node2"}, {"nil", "Node3"}, {"nil"}},
		},
		{
			name: "test_seo_add_nil_child",
			seoRoot: func() *SEO {
				rootSeo := &SEO{name: "Root"}
				var nilSeo *SEO
				node1 := &SEO{name: "Node1"}
				node2 := &SEO{name: "Node2"}
				rootSeo.AppendChildren(node1.AppendChildren(node2), nilSeo)
				return rootSeo
			}(),
			expected: [][]string{{"Root"}, {"Node1"}, {"Node2"}, {"nil"}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
		})
	}
}

func TestSEO_RemoveSelf(t *testing.T) {
	cases := []struct {
		name     string
		seoRoot  *SEO
		expected [][]string
	}{
		{
			name: "test_remove_node",
			seoRoot: func() *SEO {
				seoRoot := &SEO{name: "Root"}
				node1 := &SEO{name: "level1-1"}
				node2 := &SEO{name: "level1-2"}
				node3 := &SEO{name: "level2-1"}
				node4 := &SEO{name: "level2-2"}
				seoRoot.AppendChildren(node1.AppendChildren(node3, node4), node2)
				node1.RemoveSelf()
				return seoRoot
			}(),
			expected: [][]string{{"Root"}, {"level1-2", "level2-1", "level2-2"}, {"nil", "nil", "nil"}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			check(c.seoRoot, c.expected, t)
		})
	}
}

// check function checks if the structure of seoRoot conforms to the expected shape
// after level-order traversal
func check(seoRoot *SEO, expected [][]string, t *testing.T) {
	var que []*SEO
	que = append(que, seoRoot)
	level := 0
	for len(que) > 0 {
		curLen := len(que)
		if curLen != len(expected[level]) {
			t.Errorf("The number of nodes in the current level does not meet the expected quantity.")
		}
		i := 0
		for i < curLen {
			cur := que[0]
			expectedName := expected[level][i]
			if cur == nil {
				if expectedName != "nil" {
					t.Errorf("actual: %v, expected: %v", "nil", expectedName)
				}
			} else {
				if expectedName != cur.name {
					t.Errorf("actual: %v, expected: %v", cur.name, expectedName)
				}
				if cur.children == nil {
					que = append(que, nil)
				} else {
					for _, child := range cur.children {
						que = append(que, child)
					}
				}
			}
			que = que[1:]
			i++
		}
		level++
	}
}
