// Copyright (c) 2016, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package sh

import (
	"reflect"
	"strings"
	"testing"
)

func lits(strs ...string) []node {
	l := make([]node, 0, len(strs))
	for _, s := range strs {
		l = append(l, lit{val: s})
	}
	return l
}

var tests = []struct {
	ins  []string
	want interface{}
}{
	{
		ins:  []string{"", " ", "\n"},
		want: nil,
	},
	{
		ins:  []string{"# foo", "# foo\n"},
		want: comment{text: " foo"},
	},
	{
		ins:  []string{"foo", "foo ", " foo"},
		want: command{args: lits("foo")},
	},
	{
		ins: []string{"foo; bar", "foo; bar;", "\nfoo\nbar\n"},
		want: []node{
			command{args: lits("foo")},
			command{args: lits("bar")},
		},
	},
	{
		ins:  []string{"foo a b", " foo  a  b ", "foo \\\n a b"},
		want: command{args: lits("foo", "a", "b")},
	},
	{
		ins: []string{"( foo; )", "(foo;)", "(\nfoo\n)"},
		want: subshell{stmts: []node{
			command{args: lits("foo")},
		}},
	},
	{
		ins: []string{"{ foo; }", "{foo;}", "{\nfoo\n}"},
		want: block{stmts: []node{
			command{args: lits("foo")},
		}},
	},
	{
		ins: []string{
			"if a; then b; fi",
			"if a\nthen\nb\nfi",
		},
		want: ifStmt{
			cond: command{args: lits("a")},
			thenStmts: []node{
				command{args: lits("b")},
			},
		},
	},
	{
		ins: []string{
			"if a; then b; else c; fi",
			"if a\nthen b\nelse\nc\nfi",
		},
		want: ifStmt{
			cond: command{args: lits("a")},
			thenStmts: []node{
				command{args: lits("b")},
			},
			elseStmts: []node{
				command{args: lits("c")},
			},
		},
	},
	{
		ins: []string{
			"if a; then a; elif b; then b; elif c; then c; else d; fi",
			"if a\nthen a\nelif b\nthen b\nelif c\nthen c\nelse\nd\nfi",
		},
		want: ifStmt{
			cond: command{args: lits("a")},
			thenStmts: []node{
				command{args: lits("a")},
			},
			elifs: []node{
				elif{cond: command{args: lits("b")},
					thenStmts: []node{
						command{args: lits("b")},
					}},
				elif{cond: command{args: lits("c")},
					thenStmts: []node{
						command{args: lits("c")},
					}},
			},
			elseStmts: []node{
				command{args: lits("d")},
			},
		},
	},
	{
		ins: []string{"while a; do b; done", "while a\ndo\nb\ndone"},
		want: whileStmt{
			cond: command{args: lits("a")},
			doStmts: []node{
				command{args: lits("b")},
			},
		},
	},
	{
		ins:  []string{"echo ' ' \"foo bar\""},
		want: command{args: lits("echo", "' '", "\"foo bar\"")},
	},
	{
		ins:  []string{"$a ${b} s{s s=s"},
		want: command{args: lits("$a", "${b}", "s{s", "s=s")},
	},
	{
		ins: []string{"foo && bar", "foo&&bar", "foo &&\nbar"},
		want: binaryExpr{
			op: "&&",
			X:  command{args: lits("foo")},
			Y:  command{args: lits("bar")},
		},
	},
	{
		ins: []string{"foo || bar", "foo||bar", "foo ||\nbar"},
		want: binaryExpr{
			op: "||",
			X:  command{args: lits("foo")},
			Y:  command{args: lits("bar")},
		},
	},
	{
		ins: []string{"foo && bar || else"},
		want: binaryExpr{
			op: "&&",
			X:  command{args: lits("foo")},
			Y: binaryExpr{
				op: "||",
				X:  command{args: lits("bar")},
				Y:  command{args: lits("else")},
			},
		},
	},
	{
		ins: []string{"foo | bar"},
		want: binaryExpr{
			op: "|",
			X:  command{args: lits("foo")},
			Y:  command{args: lits("bar")},
		},
	},
	{
		ins: []string{"foo | bar | extra"},
		want: binaryExpr{
			op: "|",
			X:  command{args: lits("foo")},
			Y: binaryExpr{
				op: "|",
				X:  command{args: lits("bar")},
				Y:  command{args: lits("extra")},
			},
		},
	},
	{
		ins: []string{
			"foo() { a; b; }",
			"foo() {\na\nb\n}",
			"foo ( ) {\na\nb\n}",
		},
		want: funcDecl{
			name: lit{val: "foo"},
			body: block{stmts: []node{
				command{args: lits("a")},
				command{args: lits("b")},
			}},
		},
	},
	{
		ins: []string{
			"foo >a >>b <c",
			"foo > a >> b < c",
		},
		want: command{
			args: []node{
				lit{val: "foo"},
				redirect{op: ">", obj: lit{val: "a"}},
				redirect{op: ">>", obj: lit{val: "b"}},
				redirect{op: "<", obj: lit{val: "c"}},
			},
		},
	},
}

func wantedProg(v interface{}) (p prog) {
	switch x := v.(type) {
	case []node:
		p.stmts = x
	case node:
		p.stmts = append(p.stmts, x)
	}
	return
}

func TestParseAST(t *testing.T) {
	for _, c := range tests {
		want := wantedProg(c.want)
		for _, in := range c.ins {
			r := strings.NewReader(in)
			got, err := parse(r, "")
			if err != nil {
				t.Fatalf("Unexpected error in %q: %v", in, err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("AST mismatch in %q\nwant: %s\ngot:  %s\ndumps:\n%#v\n%#v",
					in, want.String(), got.String(), want, got)
			}
		}
	}
}

func TestPrintAST(t *testing.T) {
	for _, c := range tests {
		in := wantedProg(c.want)
		want := c.ins[0]
		got := in.String()
		if got != want {
			t.Fatalf("AST print mismatch\nwant: %s\ngot:  %s",
				want, got)
		}
	}
}