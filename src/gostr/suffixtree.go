package gostr

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// -- Substrings represented as intervals -----------------
type interval struct {
	i, j int
}

func (r interval) length() int {
	return r.j - r.i
}

// Chump off the first k letters of an interval
func (r interval) chump(k int) interval {
	return interval{r.i + k, r.j}
}

func (r interval) prefix(k int) interval {
	return interval{r.i, r.i + k}
}

func (r interval) substr(x string) string {
	return x[r.i:r.j]
}

// STNode represents the nodes in a suffix tree.
type STNode interface {
	// This is an opague type, so the interface
	// is interly private. There are functions below
	// for the public interface
	getInterval() interval
	chumpInterval(int)

	getParent() STNode
	setParent(parent STNode)

	getChildren(chan<- STNode)
	leafLabels(chan<- int)

	isLeaf() bool

	toDot(x string, w io.Writer)

	edgeLabel(x string) string
}

// -- Public interface to STNodes ------

func IsLeaf(n STNode) bool {
	return n.isLeaf()
}

func Parent(n STNode) STNode {
	return n.getParent()
}

func Children(n STNode) <-chan STNode {
	outchan := make(chan STNode)
	go func() {
		n.getChildren(outchan)
		close(outchan)
	}()
	return outchan
}

func ToDot(n STNode, x string, w io.Writer) {
	n.toDot(x, w)
}

func LeafLabels(n STNode) <-chan int {
	outres := make(chan int)
	go func() {
		n.leafLabels(outres)
		close(outres)
	}()
	return outres
}

func EdgeLabel(n STNode, x string) string {
	return n.edgeLabel(x)
}

// Data both in inner STNodes and in leaf-STNodes
type sharedNode struct {
	interval
	parent STNode
}

func (n *sharedNode) getParent() STNode {
	return n.parent
}

func (n *sharedNode) setParent(parent STNode) {
	n.parent = parent
}

func (n *sharedNode) getInterval() interval {
	return n.interval
}

func (n *sharedNode) chumpInterval(i int) {
	n.i += i
}

func (n *sharedNode) edgeLabel(x string) string {
	return n.interval.substr(x)
}

type innerNode struct {
	sharedNode
	suffixLink  STNode
	children    map[byte]STNode
	sortedEdges *[]byte // Cached sorted edges for lexicographic output
}

func newInner(inter interval) *innerNode {
	return &innerNode{
		sharedNode: sharedNode{interval: inter},
		children:   map[byte]STNode{}}
}

func (n *innerNode) isLeaf() bool {
	return false
}

func sortChildren(n *innerNode) *[]byte {
	edges := []byte{}
	for k := range n.children {
		edges = append(edges, k)
	}
	sort.Slice(edges, func(i, j int) bool {
		return edges[i] < edges[j]
	})
	return &edges
}

func (n *innerNode) getChildren(res chan<- STNode) {
	if n.sortedEdges == nil {
		n.sortedEdges = sortChildren(n)
	}
	for _, edge := range *n.sortedEdges {
		res <- n.children[edge]
	}
}

func (n *innerNode) addChild(child STNode, x string) {
	if n.sortedEdges != nil {
		panic("The edges should never be sorted while we construct the tree.")
	}
	n.children[x[child.getInterval().i]] = child
	child.setParent(n)
}

func ReplaceSentinel(x string) string {
	// Need this one for readable output
	return strings.ReplaceAll(x, "\x00", "†")
}

func (n *innerNode) toDot(x string, w io.Writer) {
	if n.parent == nil {
		// Root
		fmt.Fprintf(w, "\"%p\"[label=\"\", shape=circle, style=filled, fillcolor=grey]\n", n)
	} else {
		fmt.Fprintf(w, "\"%p\" -> \"%p\"[label=\"%s\"]\n",
			n.getParent(), n, ReplaceSentinel(n.edgeLabel(x)))
		fmt.Fprintf(w, "\"%p\"[shape=point]\n", n)
	}
	for _, child := range n.children {
		child.toDot(x, w)
	}
}

func (n *innerNode) leafLabels(outchan chan<- int) {
	for child := range Children(n) {
		child.leafLabels(outchan)
	}
}

type leafNode struct {
	sharedNode
	leafIdx int
}

func newLeaf(idx int, inter interval) *leafNode {
	return &leafNode{
		sharedNode: sharedNode{interval: inter},
		leafIdx:    idx}
}

func (n *leafNode) isLeaf() bool {
	return true
}

func (n *leafNode) getChildren(res chan<- STNode) {
	// No children in a leaf
}

func (n *leafNode) toDot(x string, w io.Writer) {
	fmt.Fprintf(w, "\"%p\" -> \"%p\"[label=\"%s\"]\n",
		n.parent, n, ReplaceSentinel(n.edgeLabel(x)))
	fmt.Fprintf(w, "\"%p\"[label=%d]\n", n, n.leafIdx)
}

func (n *leafNode) leafLabels(outchan chan<- int) {
	outchan <- n.leafIdx
}

// -- Suffix tree --------------------------

type SuffixTree struct {
	String string
	Root   STNode
}

func (st *SuffixTree) ToDot(w io.Writer) {
	fmt.Fprintln(w, "digraph {")
	ToDot(st.Root, st.String, w)
	fmt.Fprintln(w, "}")
}

func (st *SuffixTree) Search(p string) <-chan int {
	n, depth, y := sscan(st.Root, interval{0, len(p)}, st.String, p)
	if depth == y.length() {
		return LeafLabels(n)
	} else {
		res := make(chan int)
		close(res) // No results
		return res
	}
}

// -- Construction algorithms --------------------------

// This function doesn't really belong with suffix trees,
// but this is where I need it...
func min(vars ...int) int {
	m := vars[0]
	for _, n := range vars {
		if n < m {
			m = n
		}
	}
	return m
}

func lenSharedPrefix(i1, i2 interval, x, y string) int {
	i, n := 0, min(i1.length(), i2.length())
	for ; i < n; i++ {
		if x[i1.i+i] != y[i2.i+i] {
			break
		}
	}
	return i
}

// x is the underlying strings for nodes, y is the string
// for inter (which when we construct is also x, but when we
// search it is likely another string).
func sscan(n STNode, inter interval, x, y string) (STNode, int, interval) {
	if inter.length() == 0 {
		return n, 0, inter
	}
	// If we scan on a node, it is an inner node.
	v, ok := n.(*innerNode).children[y[inter.i]]
	if !ok {
		return n, 0, inter
	}
	i := lenSharedPrefix(v.getInterval(), inter, x, y)
	if i == inter.length() || i < v.getInterval().length() {
		return v, i, inter
	}
	// Continue from v (exploiting tail call optimisation)
	return sscan(v, inter.chump(i), x, y)
}

func breakEdge(n STNode, depth, leafidx int, y interval, x string) *leafNode {
	if n.getParent() == nil {
		panic("A node must have a parent when we break its edge.")
	}
	new_node := newInner(n.getInterval().prefix(depth))
	n.getParent().(*innerNode).addChild(new_node, x)
	new_leaf := newLeaf(leafidx, y)
	n.chumpInterval(depth)
	new_node.addChild(new_leaf, x)
	new_node.addChild(n, x)
	return new_leaf
}

func NaiveST(x string) SuffixTree {
	// Add sentinel
	x += "\x00"
	root := newInner(interval{0, 0})
	for i := 0; i < len(x); i++ {
		v, j, y := sscan(root, interval{i, len(x)}, x, x)
		if j == 0 {
			// A mismatch when we try to leave a node
			// means that it is an inner node
			v.(*innerNode).addChild(newLeaf(i, y), x)
		} else {
			breakEdge(v, j, i, y.chump(j), x)
		}
	}
	return SuffixTree{x, root}
}

func fscan(n STNode, inter interval, x string) (STNode, int, interval) {
	if inter.length() == 0 {
		return n, 0, inter
	}
	// If we scan on a node, it is an inner node
	v, ok := n.(*innerNode).children[x[inter.i]]
	if !ok {
		panic("With fscan there should always be an out-edge")
	}
	i := min(v.getInterval().length(), inter.length())
	if i == inter.length() {
		return v, i, inter
	}
	// Continue from v (exploiting tail call optimisation)
	return fscan(v, inter.chump(i), x)
}

func (v *sharedNode) suffix() interval {
	// If v's parent is the root, chop
	// off one index
	if v.parent.getParent() == nil {
		return v.chump(1)
	} else {
		return v.interval
	}
}

func McCreight(x string) SuffixTree {
	x += "\x00"
	root := newInner(interval{0, 0})
	root.suffixLink = root
	currLeaf := newLeaf(0, interval{0, len(x)})
	root.addChild(currLeaf, x)

	// The bits of the suffix we need to search for
	var y, z interval
	// ynode is the node we get to when searching for y
	var ynode STNode

	for i := 1; i < len(x); i++ {
		p := currLeaf.getParent().(*innerNode)

		if p.suffixLink != nil {
			// We don't need y here, just z and ynode
			z = currLeaf.suffix()
			ynode = p.suffixLink
		} else {
			pp := p.parent.(*innerNode)
			// this time we need to search in both y and z
			y = p.suffix()
			z = currLeaf.interval
			ynode, depth, _ := fscan(pp.suffixLink, y, x)
			if depth < ynode.getInterval().length() {
				// ended on an edge
				currLeaf = breakEdge(ynode, depth, i, z, x)
				p.suffixLink = currLeaf.parent
				continue // Go to next suffix, we are done here
			}

			// Remember p's suffix link for later...
			p.suffixLink = ynode
		}

		// This is the slow scan part, from ynode and the rest
		// of the suffix, which is z.
		n, depth, w := sscan(ynode, z, x, x)
		if depth == 0 {
			// Landed on a node
			currLeaf = newLeaf(i, w)
			n.(*innerNode).addChild(currLeaf, x)
		} else {
			// Landed on an edge
			currLeaf = breakEdge(n, depth, i, w.chump(depth), x)
		}
	}
	return SuffixTree{x, root}
}
