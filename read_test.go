package gcfg

import (
	"bytes"
	"encoding"
	"fmt"
	"math/big"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"errors"

	"github.com/go-git/gcfg/v2/token"
)

const (
	// 64 spaces
	sp64 = "                                                                "
	// 512 spaces
	sp512 = sp64 + sp64 + sp64 + sp64 + sp64 + sp64 + sp64 + sp64
	// 4096 spaces
	sp4096 = sp512 + sp512 + sp512 + sp512 + sp512 + sp512 + sp512 + sp512
)

type cBasic struct {
	Section           cBasicS1
	Hyphen_In_Section cBasicS2
	unexported        cBasicS1
	Exported          cBasicS3
	TagName           cBasicS1 `gcfg:"tag-name"`
}
type cBasicS1 struct {
	Name  string
	Int   int
	PName *string
}
type cBasicS2 struct {
	Hyphen_In_Name string
}
type cBasicS3 struct {
	unexported string
}

type nonMulti []string

type unmarshalable string

func (u *unmarshalable) UnmarshalText(text []byte) error {
	s := string(text)
	if s == "error" {
		return fmt.Errorf("%s", s)
	}
	*u = unmarshalable(s)
	return nil
}

var _ encoding.TextUnmarshaler = new(unmarshalable)

type cUni struct {
	X甲       cUniS1
	XSection cUniS2
}
type cUniS1 struct {
	X乙 string
}
type cUniS2 struct {
	XName string
}

type cMulti struct {
	M1 cMultiS1
	M2 cMultiS2
	M3 cMultiS3
}
type cMultiS1 struct{ Multi []string }
type cMultiS2 struct{ NonMulti nonMulti }
type cMultiS3 struct{ PMulti *[]string }

type cSubs struct{ Sub map[string]*cSubsS1 }
type cSubsS1 struct{ Name string }

type cBool struct{ Section cBoolS1 }
type cBoolS1 struct{ Bool bool }

type cTxUnm struct{ Section cTxUnmS1 }
type cTxUnmS1 struct{ Name unmarshalable }

type cNum struct {
	N1 cNumS1
	N2 cNumS2
	N3 cNumS3
}
type cNumS1 struct {
	Int    int
	IntDHO int `gcfg:",int=dho"`
	Big    *big.Int
}
type cNumS2 struct {
	MultiInt []int
	MultiBig []*big.Int
}
type cNumS3 struct{ FileMode os.FileMode }
type readtest struct {
	gcfg string
	exp  interface{}
	ok   bool
}

func newString(s string) *string           { return &s }
func newStringSlice(s ...string) *[]string { return &s }

var readtests = []struct {
	group string
	tests []readtest
}{{"scanning", []readtest{
	{"[section]\nname=value", &cBasic{Section: cBasicS1{Name: "value"}}, true},
	// hyphen in name
	{"[hyphen-in-section]\nhyphen-in-name=value", &cBasic{Hyphen_In_Section: cBasicS2{Hyphen_In_Name: "value"}}, true},
	// quoted string value
	{"[section]\nname=\"\"", &cBasic{Section: cBasicS1{Name: ""}}, true},
	{"[section]\nname=\" \"", &cBasic{Section: cBasicS1{Name: " "}}, true},
	{"[section]\nname=\"value\"", &cBasic{Section: cBasicS1{Name: "value"}}, true},
	{"[section]\nname=\" value \"", &cBasic{Section: cBasicS1{Name: " value "}}, true},
	{"\n[section]\nname=\"va ; lue\"", &cBasic{Section: cBasicS1{Name: "va ; lue"}}, true},
	{"[section]\nname=\"val\" \"ue\"", &cBasic{Section: cBasicS1{Name: "val ue"}}, true},
	{"[section]\nname=\"value", &cBasic{}, false},
	// escape sequences
	{"[section]\nname=\"va\\\\lue\"", &cBasic{Section: cBasicS1{Name: "va\\lue"}}, true},
	{"[section]\nname=\"va\\\"lue\"", &cBasic{Section: cBasicS1{Name: "va\"lue"}}, true},
	{"[section]\nname=\"va\\nlue\"", &cBasic{Section: cBasicS1{Name: "va\nlue"}}, true},
	{"[section]\nname=\"va\\tlue\"", &cBasic{Section: cBasicS1{Name: "va\tlue"}}, true},
	{"[section]\nname=x:\\\\path\\\\", &cBasic{Section: cBasicS1{Name: "x:\\path\\"}}, true},
	{"[section]\nname=\\b", &cBasic{Section: cBasicS1{Name: "\b"}}, true},
	{"\n[section]\nname=\\", &cBasic{}, false},
	{"\n[section]\nname=\\a", &cBasic{}, false},
	{"\n[section]\nname=\"val\\a\"", &cBasic{}, false},
	// a CR after a backslash only folds as part of CRLF
	{"\n[section]\nname=\"a\\\rb\"", &cBasic{}, false},
	{"\n[section]\nname=val\\", &cBasic{}, false},
	// {"\n[sub \"A\\\n\"]\nname=value", &cSubs{}, false},
	{"\n[sub \"A\\\t\"]\nname=value", &cSubs{}, false},
	// broken line
	// {"[section]\nname=value \\\n value", &cBasic{Section: cBasicS1{Name: "value  value"}}, true},
	// {"[section]\nname=\"value \\\n value\"", &cBasic{}, false},
}}, {"scanning:bom", []readtest{
	{"\uFEFF[section]\nname=value", &cBasic{Section: cBasicS1{Name: "value"}}, true},
	// only a complete BOM, and only at the very start, is skipped
	{"\xEF[section]\nname=value", &cBasic{}, false},
	{"\xEF\xBB[section]\nname=value", &cBasic{}, false},
	{"\uFEFF\uFEFF[section]\nname=value", &cBasic{}, false},
	{"[section]\n\uFEFFname=value", &cBasic{}, false},
}}, {"scanning:whitespace", []readtest{
	{" \n[section]\nname=value", &cBasic{Section: cBasicS1{Name: "value"}}, true},
	{" [section]\nname=value", &cBasic{Section: cBasicS1{Name: "value"}}, true},
	{"\t[section]\nname=value", &cBasic{Section: cBasicS1{Name: "value"}}, true},
	{"[ section]\nname=value", &cBasic{Section: cBasicS1{Name: "value"}}, true},
	{"[section ]\nname=value", &cBasic{Section: cBasicS1{Name: "value"}}, true},
	{"[section]\n name=value", &cBasic{Section: cBasicS1{Name: "value"}}, true},
	{"[section]\nname =value", &cBasic{Section: cBasicS1{Name: "value"}}, true},
	{"[section]\nname= value", &cBasic{Section: cBasicS1{Name: "value"}}, true},
	{"[section]\nname=value ", &cBasic{Section: cBasicS1{Name: "value"}}, true},
	{"[section]\r\nname=value", &cBasic{Section: cBasicS1{Name: "value"}}, true},
	{"[section]\r\nname=value\r\n", &cBasic{Section: cBasicS1{Name: "value"}}, true},
	{";cmnt\r\n[section]\r\nname=value\r\n", &cBasic{Section: cBasicS1{Name: "value"}}, true},
	// long lines
	{sp4096 + "[section]\nname=value\n", &cBasic{Section: cBasicS1{Name: "value"}}, true},
	{"[" + sp4096 + "section]\nname=value\n", &cBasic{Section: cBasicS1{Name: "value"}}, true},
	{"[section" + sp4096 + "]\nname=value\n", &cBasic{Section: cBasicS1{Name: "value"}}, true},
	{"[section]" + sp4096 + "\nname=value\n", &cBasic{Section: cBasicS1{Name: "value"}}, true},
	{"[section]\n" + sp4096 + "name=value\n", &cBasic{Section: cBasicS1{Name: "value"}}, true},
	{"[section]\nname" + sp4096 + "=value\n", &cBasic{Section: cBasicS1{Name: "value"}}, true},
	{"[section]\nname=" + sp4096 + "value\n", &cBasic{Section: cBasicS1{Name: "value"}}, true},
	{"[section]\nname=value\n" + sp4096, &cBasic{Section: cBasicS1{Name: "value"}}, true},
}}, {"scanning:comments", []readtest{
	{"; cmnt\n[section]\nname=value", &cBasic{Section: cBasicS1{Name: "value"}}, true},
	{"# cmnt\n[section]\nname=value", &cBasic{Section: cBasicS1{Name: "value"}}, true},
	{" ; cmnt\n[section]\nname=value", &cBasic{Section: cBasicS1{Name: "value"}}, true},
	{"\t; cmnt\n[section]\nname=value", &cBasic{Section: cBasicS1{Name: "value"}}, true},
	{"\n[section]; cmnt\nname=value", &cBasic{Section: cBasicS1{Name: "value"}}, true},
	{"\n[section] ; cmnt\nname=value", &cBasic{Section: cBasicS1{Name: "value"}}, true},
	{"\n[section]\nname=value; cmnt", &cBasic{Section: cBasicS1{Name: "value"}}, true},
	{"\n[section]\nname=value ; cmnt", &cBasic{Section: cBasicS1{Name: "value"}}, true},
	{"\n[section]\nname=\"value\" ; cmnt", &cBasic{Section: cBasicS1{Name: "value"}}, true},
	{"\n[section]\nname=value ; \"cmnt", &cBasic{Section: cBasicS1{Name: "value"}}, true},
	{"\n[section]\nname=\"va ; lue\" ; cmnt", &cBasic{Section: cBasicS1{Name: "va ; lue"}}, true},
	{"\n[section]\nname=; cmnt", &cBasic{Section: cBasicS1{Name: ""}}, true},
}}, {"scanning:subsections", []readtest{
	{"\n[sub \"A\"]\nname=value", &cSubs{map[string]*cSubsS1{"A": {"value"}}}, true},
	{"\n[sub \"b\"]\nname=value", &cSubs{map[string]*cSubsS1{"b": {"value"}}}, true},
	{"\n[sub \"A\\\\\"]\nname=value", &cSubs{map[string]*cSubsS1{"A\\": {"value"}}}, true},
	{"\n[sub \"A\\\"\"]\nname=value", &cSubs{map[string]*cSubsS1{"A\"": {"value"}}}, true},
}}, {"syntax", []readtest{
	// invalid line
	{"\n[section]\n=", &cBasic{}, false},
	// no section
	{"name=value", &cBasic{}, false},
	// empty section
	{"\n[]\nname=value", &cBasic{}, false},
	// empty subsection name
	{"\n[sub \"\"]\nname=value", &cSubs{Sub: map[string]*cSubsS1{"": {"value"}}}, true},
}}, {"setting", []readtest{
	{"[section]\nname=value", &cBasic{Section: cBasicS1{Name: "value"}}, true},
	// pointer
	{"[section]", &cBasic{Section: cBasicS1{PName: nil}}, true},
	{"[section]\npname=value", &cBasic{Section: cBasicS1{PName: newString("value")}}, true},
	{"[m3]", &cMulti{M3: cMultiS3{PMulti: nil}}, true},
	{"[m3]\npmulti", &cMulti{M3: cMultiS3{PMulti: newStringSlice()}}, true},
	{"[m3]\npmulti=value", &cMulti{M3: cMultiS3{PMulti: newStringSlice("value")}}, true},
	{"[m3]\npmulti=value1\npmulti=value2", &cMulti{M3: cMultiS3{PMulti: newStringSlice("value1", "value2")}}, true},
	// section name not matched
	{"\n[nonexistent]\nname=value", &cBasic{}, false},
	// subsection name not matched
	{"\n[section \"nonexistent\"]\nname=value", &cBasic{}, false},
	// variable name not matched
	{"\n[section]\nnonexistent=value", &cBasic{}, false},
	// hyphen in name
	{"[hyphen-in-section]\nhyphen-in-name=value", &cBasic{Hyphen_In_Section: cBasicS2{Hyphen_In_Name: "value"}}, true},
	// ignore unexported fields
	{"[unexported]\nname=value", &cBasic{}, false},
	{"[exported]\nunexported=value", &cBasic{}, false},
	// 'X' prefix for non-upper/lower-case letters
	{"[甲]\n乙=丙", &cUni{X甲: cUniS1{X乙: "丙"}}, true},
	//{"[section]\nxname=value", &cBasic{XSection: cBasicS4{XName: "value"}}, false},
	//{"[xsection]\nname=value", &cBasic{XSection: cBasicS4{XName: "value"}}, false},
	// name specified as struct tag
	{"[tag-name]\nname=value", &cBasic{TagName: cBasicS1{Name: "value"}}, true},
	// empty subsections
	{"\n[sub \"A\"]\n[sub \"B\"]", &cSubs{map[string]*cSubsS1{"A": {}, "B": {}}}, true},
}}, {"multivalue", []readtest{
	// unnamed slice type: treat as multi-value
	{"\n[m1]", &cMulti{M1: cMultiS1{}}, true},
	{"\n[m1]\nmulti=value", &cMulti{M1: cMultiS1{[]string{"value"}}}, true},
	{"\n[m1]\nmulti=value1\nmulti=value2", &cMulti{M1: cMultiS1{[]string{"value1", "value2"}}}, true},
	// "blank" empties multi-valued slice -- here same result as above
	{"\n[m1]\nmulti\nmulti=value1\nmulti=value2", &cMulti{M1: cMultiS1{[]string{"value1", "value2"}}}, true},
	// named slice type: do not treat as multi-value
	{"\n[m2]", &cMulti{}, true},
	{"\n[m2]\nmulti=value", &cMulti{}, false},
	{"\n[m2]\nmulti=value1\nmulti=value2", &cMulti{}, false},
}}, {"type:string", []readtest{
	{"[section]\nname=value", &cBasic{Section: cBasicS1{Name: "value"}}, true},
	{"[section]\nname=", &cBasic{Section: cBasicS1{Name: ""}}, true},
}}, {"type:bool", []readtest{
	// explicit values
	{"[section]\nbool=true", &cBool{cBoolS1{true}}, true},
	{"[section]\nbool=yes", &cBool{cBoolS1{true}}, true},
	{"[section]\nbool=on", &cBool{cBoolS1{true}}, true},
	{"[section]\nbool=1", &cBool{cBoolS1{true}}, true},
	{"[section]\nbool=tRuE", &cBool{cBoolS1{true}}, true},
	{"[section]\nbool=false", &cBool{cBoolS1{false}}, true},
	{"[section]\nbool=no", &cBool{cBoolS1{false}}, true},
	{"[section]\nbool=off", &cBool{cBoolS1{false}}, true},
	{"[section]\nbool=0", &cBool{cBoolS1{false}}, true},
	{"[section]\nbool=NO", &cBool{cBoolS1{false}}, true},
	// "blank" value handled as true
	{"[section]\nbool", &cBool{cBoolS1{true}}, true},
	// bool parse errors
	{"[section]\nbool=maybe", &cBool{}, false},
	{"[section]\nbool=t", &cBool{}, false},
	{"[section]\nbool=truer", &cBool{}, false},
	{"[section]\nbool=2", &cBool{}, false},
	{"[section]\nbool=-1", &cBool{}, false},
}}, {"type:numeric", []readtest{
	{"[section]\nint=0", &cBasic{Section: cBasicS1{Int: 0}}, true},
	{"[section]\nint=1", &cBasic{Section: cBasicS1{Int: 1}}, true},
	{"[section]\nint=-1", &cBasic{Section: cBasicS1{Int: -1}}, true},
	{"[section]\nint=0.2", &cBasic{}, false},
	{"[section]\nint=1e3", &cBasic{}, false},
	// primitive [u]int(|8|16|32|64) and big.Int is parsed as dec or hex (not octal)
	{"[n1]\nint=010", &cNum{N1: cNumS1{Int: 10}}, true},
	{"[n1]\nint=0x10", &cNum{N1: cNumS1{Int: 0x10}}, true},
	{"[n1]\nbig=1", &cNum{N1: cNumS1{Big: big.NewInt(1)}}, true},
	{"[n1]\nbig=0x10", &cNum{N1: cNumS1{Big: big.NewInt(0x10)}}, true},
	{"[n1]\nbig=010", &cNum{N1: cNumS1{Big: big.NewInt(10)}}, true},
	{"[n2]\nmultiint=010", &cNum{N2: cNumS2{MultiInt: []int{10}}}, true},
	{"[n2]\nmultibig=010", &cNum{N2: cNumS2{MultiBig: []*big.Int{big.NewInt(10)}}}, true},
	// set parse mode for int types via struct tag
	{"[n1]\nintdho=010", &cNum{N1: cNumS1{IntDHO: 010}}, true},
	// octal allowed for named type
	{"[n3]\nfilemode=0777", &cNum{N3: cNumS3{FileMode: 0777}}, true},
}}, {"type:textUnmarshaler", []readtest{
	{"[section]\nname=value", &cTxUnm{Section: cTxUnmS1{Name: "value"}}, true},
	{"[section]\nname=error", &cTxUnm{}, false},
}},
}

func TestReadStringInto(t *testing.T) {
	for _, tg := range readtests {
		for i, tt := range tg.tests {
			id := fmt.Sprintf("%s:%d", tg.group, i)
			t.Run(id+tt.gcfg, func(t *testing.T) {
				testRead(t, id, tt)
			})
		}
	}
}

func TestReadStringIntoMultiBlankPreset(t *testing.T) {
	tt := readtest{"\n[m1]\nmulti\nmulti=value1\nmulti=value2", &cMulti{M1: cMultiS1{[]string{"value1", "value2"}}}, true}
	cfg := &cMulti{M1: cMultiS1{[]string{"preset1", "preset2"}}}
	testReadInto(t, "multi:blank", tt, cfg)
}

func testRead(t *testing.T, id string, tt readtest) {
	// get the type of the expected result
	restyp := reflect.TypeOf(tt.exp).Elem()
	// create a new instance to hold the actual result
	res := reflect.New(restyp).Interface()
	testReadInto(t, id, tt, res)
}

func testReadInto(t *testing.T, id string, tt readtest, res interface{}) {
	err := ReadStringInto(res, tt.gcfg)
	if tt.ok {
		if err != nil {
			t.Errorf("%s fail: got error %v, wanted ok", id, err)
			return
		} else if !reflect.DeepEqual(res, tt.exp) {
			t.Errorf("%s fail: got value %#v, wanted value %#v", id, res, tt.exp)
			return
		}
		if !testing.Short() {
			t.Logf("%s pass: got value %#v", id, res)
		}
	} else { // !tt.ok
		if err == nil {
			t.Errorf("%s fail: got value %#v, wanted error", id, res)
			return
		}
		if !testing.Short() {
			t.Logf("%s pass: got error %v", id, err)
		}
	}
}

func TestReadFileInto(t *testing.T) {
	res := &struct{ Section struct{ Name string } }{}
	err := ReadFileInto(res, "testdata/gcfg_test.gcfg")
	if err != nil {
		t.Error(err)
	}
	if "value" != res.Section.Name {
		t.Errorf("got %q, wanted %q", res.Section.Name, "value")
	}
}

func TestReadFileIntoUnicode(t *testing.T) {
	res := &struct{ X甲 struct{ X乙 string } }{}
	err := ReadFileInto(res, "testdata/gcfg_unicode_test.gcfg")
	if err != nil {
		t.Error(err)
	}
	if "丙" != res.X甲.X乙 {
		t.Errorf("got %q, wanted %q", res.X甲.X乙, "丙")
	}
}

func TestReadStringIntoSubsectDefaults(t *testing.T) {
	type subsect struct {
		Color       string
		Orientation string
	}
	res := &struct {
		Default_Profile subsect
		Profile         map[string]*subsect
	}{Default_Profile: subsect{Color: "green"}}
	cfg := `
	[profile "one"]
	orientation = left`
	err := ReadStringInto(res, cfg)
	if err != nil {
		t.Error(err)
	}
	if res.Profile["one"].Color != "green" {
		t.Errorf("got %q; want %q", res.Profile["one"].Color, "green")
	}
}

func TestReadStringIntoExtraData(t *testing.T) {
	res := &struct {
		Section struct {
			Name string
		}
	}{}
	cfg := `
	[section]
	name = value
	name2 = value2`
	err := FatalOnly(ReadStringInto(res, cfg))
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if res.Section.Name != "value" {
		t.Errorf("res.Section.Name=%q; want %q", res.Section.Name, "value")
	}
}

func TestReadWithCallback(t *testing.T) {
	results := [][]string{}
	cb := func(s string, ss string, k string, v string, bv bool) error {
		results = append(results, []string{s, ss, k, v, strconv.FormatBool(bv)})
		return nil
	}
	text := `
	[sect1]
	key1=value1
	[sect1 "subsect1"]
	key2=value2
	key3=value3
	key4
	key5=
	[sect1 "subsect2"]
	[sect2]
	[sect3]
    foo = "!f(){ \
	echo hello; \
	};f"
	`
	expected := [][]string{
		{"sect1", "", "", "", "true"},
		{"sect1", "", "key1", "value1", "false"},
		{"sect1", "subsect1", "", "", "true"},
		{"sect1", "subsect1", "key2", "value2", "false"},
		{"sect1", "subsect1", "key3", "value3", "false"},
		{"sect1", "subsect1", "key4", "", "true"},
		{"sect1", "subsect1", "key5", "", "false"},
		{"sect1", "subsect2", "", "", "true"},
		{"sect2", "", "", "", "true"},
		{"sect3", "", "", "", "true"},
		{"sect3", "", "foo", "!f(){ \techo hello; \t};f", "false"},
	}
	err := ReadWithCallback(bytes.NewReader([]byte(text)), cb)
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(results, expected) {
		t.Errorf("expected %+v, got %+v", expected, results)
	}

	i := 0
	expectedErr := errors.New("FATAL ERROR")
	results = [][]string{}
	cbWithError := func(s string, ss string, k string, v string, bv bool) error {
		results = append(results, []string{s, ss, k, v, strconv.FormatBool(bv)})
		i += 1
		if i == 3 {
			return expectedErr
		}
		return nil
	}
	err = ReadWithCallback(bytes.NewReader([]byte(text)), cbWithError)
	if err != expectedErr {
		t.Errorf("expected error: %+v", err)
	}
	if !reflect.DeepEqual(results, expected[:3]) {
		t.Errorf("expected %+v, got %+v", expected, results[:3])
	}
}

// The expected values are what git config --list reports for the same text.
func TestReadWithCallbackMatchesGit(t *testing.T) {
	for _, tt := range []struct {
		name     string
		text     string
		expected [][]string
	}{{
		name: "utf-8 BOM",
		text: "\uFEFF[core]\n\ta = 1\n",
		expected: [][]string{
			{"core", "", "", "", "true"},
			{"core", "", "a", "1", "false"},
		},
	}, {
		name: "continuation outside quotes",
		text: "[core]\n\ta = first \\\n\tsecond\n",
		expected: [][]string{
			{"core", "", "", "", "true"},
			{"core", "", "a", "first \tsecond", "false"},
		},
	}, {
		name: "continuation inside quotes",
		text: "[core]\n\ta = \"q1 \\\n q2\"\n",
		expected: [][]string{
			{"core", "", "", "", "true"},
			{"core", "", "a", "q1  q2", "false"},
		},
	}, {
		name: "continuation with CRLF",
		text: "[core]\r\n\ta = \"q1 \\\r\n q2\"\r\n\tb = u1 \\\r\n u2\r\n",
		expected: [][]string{
			{"core", "", "", "", "true"},
			{"core", "", "a", "q1  q2", "false"},
			{"core", "", "b", "u1  u2", "false"},
		},
	}} {
		t.Run(tt.name, func(t *testing.T) {
			results := [][]string{}
			cb := func(s string, ss string, k string, v string, bv bool) error {
				results = append(results, []string{s, ss, k, v, strconv.FormatBool(bv)})
				return nil
			}
			if err := ReadWithCallback(bytes.NewReader([]byte(tt.text)), cb); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(results, tt.expected) {
				t.Errorf("expected %q, got %q", tt.expected, results)
			}
		})
	}
}

func TestReadWithCallback_WithError(t *testing.T) {
	results := [][]string{}
	cb := func(s string, ss string, k string, v string, bv bool) error {
		results = append(results, []string{s, ss, k, v, strconv.FormatBool(bv)})
		return nil
	}
	text := `
	[sect1]
	key1=value1
	[sect1 "subsect1"]
	key2=value2
	key3=value3
	key4
	key5=
	[sect1 "subsect2"]
	[sect2]
	`
	expected := [][]string{
		{"sect1", "", "", "", "true"},
		{"sect1", "", "key1", "value1", "false"},
		{"sect1", "subsect1", "", "", "true"},
		{"sect1", "subsect1", "key2", "value2", "false"},
		{"sect1", "subsect1", "key3", "value3", "false"},
		{"sect1", "subsect1", "key4", "", "true"},
		{"sect1", "subsect1", "key5", "", "false"},
		{"sect1", "subsect2", "", "", "true"},
		{"sect2", "", "", "", "true"},
	}
	err := ReadWithCallback(bytes.NewReader([]byte(text)), cb)
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(results, expected) {
		t.Errorf("expected %+v, got %+v", expected, results)
	}
}

func readCallbacks(t *testing.T, text string) ([][]string, error) {
	t.Helper()
	results := [][]string{}
	cb := func(s string, ss string, k string, v string, bv bool) error {
		results = append(results, []string{s, ss, k, v, strconv.FormatBool(bv)})
		return nil
	}
	err := ReadWithCallback(bytes.NewReader([]byte(text)), cb)
	return results, err
}

// Git reads a [section.subsection] header as section and subsection, both
// lowercased (config.c get_base_var). The expected values are what git
// config --list reports for the same text.
func TestReadLegacySubsectionHeader(t *testing.T) {
	for _, tt := range []struct {
		name, text string
		sect, sub  string
	}{
		{"plain", "[protocol.file]\n\tk = v\n", "protocol", "file"},
		{"uppercase", "[Protocol.File]\n\tk = v\n", "protocol", "file"},
		{"dots in subsection", "[a.b.c]\n\tk = v\n", "a", "b.c"},
		{"consecutive dots", "[a..b]\n\tk = v\n", "a", ".b"},
		{"trailing dot in subsection", "[a.b.]\n\tk = v\n", "a", "b."},
		{"hyphens and digits", "[a-1.b-2]\n\tk = v\n", "a-1", "b-2"},
		{"leading digit", "[1a.b]\n\tk = v\n", "1a", "b"},
		{"leading hyphen", "[-a.-b]\n\tk = v\n", "-a", "-b"},
		{"indented", "  \t[a.b]\n\tk = v\n", "a", "b"},
		{"trailing comment", "[a.b] ; c\n\tk = v\n", "a", "b"},
		{"trailing hash comment", "[a.b]#c\n\tk = v\n", "a", "b"},
		{"crlf", "[a.b]\r\n\tk = v\r\n", "a", "b"},
		{"bom", "\uFEFF[a.b]\n\tk = v\n", "a", "b"},
		{"no final newline", "[a.b]", "a", "b"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := readCallbacks(t, tt.text)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) == 0 || got[0][0] != tt.sect || got[0][1] != tt.sub ||
				got[0][2] != "" || got[0][4] != "true" {
				t.Errorf("header: want %q/%q, got %q", tt.sect, tt.sub, got)
			}
			if strings.Contains(tt.text, "k = v") {
				want := []string{tt.sect, tt.sub, "k", "v", "false"}
				if len(got) != 2 || !reflect.DeepEqual(got[1], want) {
					t.Errorf("option: want %q, got %q", want, got)
				}
			}
		})
	}
}

// A '[' that does not start a header is left alone.
func TestReadLegacySubsectionHeaderNotAHeader(t *testing.T) {
	for _, tt := range []struct {
		name, text string
		want       [][]string
	}{
		{"comment", "# [a.b]\n[s]\n", [][]string{{"s", "", "", "", "true"}}},
		{"value", "[s]\n\tk = [a.b]\n", [][]string{
			{"s", "", "", "", "true"}, {"s", "", "k", "[a.b]", "false"}}},
		{"continued value", "[s]\n\tk = x\\\n[a.b]\n", [][]string{
			{"s", "", "", "", "true"}, {"s", "", "k", "x[a.b]", "false"}}},
		{"quoted subsection", "[remote \"[A.B]\"]\n", [][]string{
			{"remote", "[A.B]", "", "", "true"}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := readCallbacks(t, tt.text)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("want %q, got %q", tt.want, got)
			}
		})
	}
}

// Git accepts every one of these except the first group, where it fails
// too. They are refused here, and the error points at the offending
// character or token, not at the start of the header.
//
//   - [.x], [x.] and [.] have an empty section or subsection name.
//   - Text after the header on the same line is refused for [a] as well.
//   - A mixed header such as [a.b "C"] would need a rule for which of the
//     two subsections wins.
func TestReadLegacySubsectionHeaderRefused(t *testing.T) {
	for _, tt := range []struct {
		text string
		want string // error prefix: position and message
	}{
		// git fails too
		{"[a.b c]\n", "1:3: illegal character U+002E '.'"},
		{"[a_b.c]\n", "1:3: illegal character U+005F '_'"},
		{"[a.b\n", "1:3: illegal character U+002E '.'"},
		{"[é.b]\n", "1:4: illegal character U+002E '.'"},
		// git accepts
		{"[.x]\n", "1:2: illegal character U+002E '.'"},
		{"[x.]\n", "1:3: illegal character U+002E '.'"},
		{"[.]\n", "1:2: illegal character U+002E '.'"},
		{"[a.b]garbage\n", "1:6: expected EOL, EOF, or comment"},
		{"[a.b]k = v\n", "1:6: expected EOL, EOF, or comment"},
		{"[a.b \"C\"]\n", "1:3: illegal character U+002E '.'"},
		{"[a.b \"\"]\n", "1:3: illegal character U+002E '.'"},
		{"[s]\n[a.b]x\n", "2:6: expected EOL, EOF, or comment"},
	} {
		t.Run(tt.text, func(t *testing.T) {
			_, err := readCallbacks(t, tt.text)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.HasPrefix(err.Error(), tt.want) {
				t.Errorf("want %q in error, got %q", tt.want, err)
			}
		})
	}
}

type cLegacy struct {
	Protocol map[string]*struct{ Allow string }
	Remote   map[string]*struct{ URL string }
}

func TestReadIntoLegacySubsectionHeader(t *testing.T) {
	var c cLegacy
	err := ReadStringInto(&c, "[Protocol.File]\n\tallow = never\n[protocol \"file\"]\n\tallow = user\n[remote.Origin]\n\turl = u\n")
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Protocol["file"]; got == nil || got.Allow != "user" {
		t.Errorf("protocol: %+v", c.Protocol)
	}
	if got := c.Remote["origin"]; got == nil || got.URL != "u" {
		t.Errorf("remote: %+v", c.Remote)
	}
	if _, ok := c.Remote["Origin"]; ok {
		t.Errorf("subsection must be lowercased: %+v", c.Remote)
	}
}

// Every legacy header reads as its quoted equivalent.
func TestReadLegacySubsectionHeaderEqualsQuoted(t *testing.T) {
	legacy := "[A.B.C]\n\tk = v\n[x-1.y-2] ; c\n\tk\n"
	quoted := "[a \"b.c\"]\n\tk = v\n[x-1 \"y-2\"] ; c\n\tk\n"
	got, err := readCallbacks(t, legacy)
	if err != nil {
		t.Fatal(err)
	}
	want, err := readCallbacks(t, quoted)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("want %q, got %q", want, got)
	}
}

// Sanity run over random lines built from header characters: reading must
// not panic or hang, and every line that is exactly a legacy header, as git
// get_base_var accepts it, must be reported lowercased and split at the
// first dot, while every other line must not be reported as one.
func TestReadLegacySubsectionHeaderRandom(t *testing.T) {
	const alphabet = "aZ0-.[] "
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 20000; i++ {
		b := make([]byte, rng.Intn(12))
		for j := range b {
			b[j] = alphabet[rng.Intn(len(alphabet))]
		}
		line := string(b)
		got, err := readCallbacks(t, line+"\n")
		name, ok := strings.CutPrefix(line, "[")
		if ok {
			name, ok = strings.CutSuffix(name, "]")
		}
		d := strings.IndexByte(name, '.')
		ok = ok && d > 0 && d < len(name)-1 && !strings.ContainsAny(name, "[] ")
		if !ok {
			for _, r := range got {
				if strings.Contains(line, ".") && r[1] != "" {
					t.Fatalf("%q must not read as a legacy header: %q", line, got)
				}
			}
			continue
		}
		want := [][]string{{strings.ToLower(name[:d]), strings.ToLower(name[d+1:]), "", "", "true"}}
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("%q: want %q, got %q, %v", line, want, got, err)
		}
	}
}

// Every header this package reads, git reads as the same variable. The
// reverse does not hold, see TestReadLegacySubsectionHeaderRefused.
func TestReadLegacySubsectionHeaderAgainstGit(t *testing.T) {
	gitBin, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not found")
	}
	const alphabet = "aZ0-."
	rng := rand.New(rand.NewSource(2))
	dir := t.TempDir()
	checked := 0
	for i := 0; i < 1000; i++ {
		b := make([]byte, rng.Intn(9))
		for j := range b {
			b[j] = alphabet[rng.Intn(len(alphabet))]
		}
		if len(b) > 0 && rng.Intn(5) == 0 {
			b[rng.Intn(len(b))] = " _[]"[rng.Intn(4)]
		}
		text := "[" + string(b) + "]\n\tk = v\n"
		got, err := readCallbacks(t, text)
		if err != nil {
			continue
		} else if got[0][1] == "" {
			continue // not a legacy header
		}
		file := filepath.Join(dir, "config")
		if err := os.WriteFile(file, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		out, err := exec.Command(gitBin, "config", "--file", file, "--list").Output()
		if err != nil {
			t.Fatalf("%q: read here as %q, git fails: %v", text, got, err)
		}
		r := got[len(got)-1]
		// Section names keep their case here and are matched ignoring it.
		key := strings.ToLower(r[0]) + "." + r[1] + "." + r[2]
		if want := key + "=v\n"; string(out) != want {
			t.Fatalf("%q: git reads %q, here %q", text, out, want)
		}
		checked++
	}
	if checked < 100 {
		t.Errorf("only %d inputs were compared", checked)
	}
}

// Git accepts these, with an empty section or subsection name, and they are
// refused here.
func TestReadLegacySubsectionHeaderRefusedGitAccepts(t *testing.T) {
	gitBin, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not found")
	}
	for _, text := range []string{"[.x]\n\tk = v\n", "[x.]\n\tk = v\n", "[.]\n\tk = v\n"} {
		file := filepath.Join(t.TempDir(), "config")
		if err := os.WriteFile(file, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(gitBin, "config", "--file", file, "--list").CombinedOutput(); err != nil {
			t.Errorf("%q: git fails: %s", text, out)
		}
		if _, err := readCallbacks(t, text); err == nil {
			t.Errorf("%q: expected an error", text)
		}
	}
}

type subsectionEvent struct {
	sect, sub string
	hasSub    bool
	key, val  string
	blank     bool
}

func TestReadWithOptions(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []subsectionEvent
	}{
		{"plain", "[a]\nk = 1\n", []subsectionEvent{
			{"a", "", false, "", "", true},
			{"a", "", false, "k", "1", false},
		}},
		{"quoted", "[a \"b\"]\nk\n", []subsectionEvent{
			{"a", "b", true, "", "", true},
			{"a", "b", true, "k", "", true},
		}},
		{"quoted empty", "[a \"\"]\nk = 1\n", []subsectionEvent{
			{"a", "", true, "", "", true},
			{"a", "", true, "k", "1", false},
		}},
		{"legacy", "[a.b]\nk = 1\n", []subsectionEvent{
			{"a", "b", true, "", "", true},
			{"a", "b", true, "k", "1", false},
		}},
		{"legacy trailing dot", "[a.b.]\nk = 1\n", []subsectionEvent{
			{"a", "b.", true, "", "", true},
			{"a", "b.", true, "k", "1", false},
		}},
		{"legacy double dot", "[a..b]\nk = 1\n", []subsectionEvent{
			{"a", ".b", true, "", "", true},
			{"a", ".b", true, "k", "1", false},
		}},
		{"combined", "[a.b]\nk=1\n[a \"b\"]\nk=2\n[a \"\"]\nk=3\n[a.b.]\nk=4\n[a]\nk=5\n", []subsectionEvent{
			{"a", "b", true, "", "", true},
			{"a", "b", true, "k", "1", false},
			{"a", "b", true, "", "", true},
			{"a", "b", true, "k", "2", false},
			{"a", "", true, "", "", true},
			{"a", "", true, "k", "3", false},
			{"a", "b.", true, "", "", true},
			{"a", "b.", true, "k", "4", false},
			{"a", "", false, "", "", true},
			{"a", "", false, "k", "5", false},
		}},
		{"reset on later plain header", "[x \"y\"]\nk=1\n[z]\nk=2\n[x]\nk=3\n", []subsectionEvent{
			{"x", "y", true, "", "", true},
			{"x", "y", true, "k", "1", false},
			{"z", "", false, "", "", true},
			{"z", "", false, "k", "2", false},
			{"x", "", false, "", "", true},
			{"x", "", false, "k", "3", false},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []subsectionEvent
			err := ReadWithOptions(strings.NewReader(tt.src),
				WithCallback(func(e Event) error {
					got = append(got, subsectionEvent{e.Section, e.Subsection, e.HasSubsection, e.Key, e.Value, e.BlankValue})
					return nil
				}))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func TestReadWithCallbackIgnoresSubsectionFlag(t *testing.T) {
	var got [][2]string
	err := ReadWithCallback(strings.NewReader("[x \"\"]\na = 1\n[x]\n"),
		func(s, ss, k, v string, b bool) error {
			got = append(got, [2]string{s, ss})
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %v", got)
	}
	for _, g := range got {
		if g != [2]string{"x", ""} {
			t.Errorf("got %v", g)
		}
	}
}

func TestReadWithOptionsRefusesEmptyLegacyName(t *testing.T) {
	err := ReadWithOptions(strings.NewReader("[x.]\nk = 1\n"),
		WithCallback(func(Event) error { return nil }))
	if err == nil {
		t.Error("expected an error for [x.]")
	}
}

func TestReadWithOptionsWithoutCallback(t *testing.T) {
	nop := func(string, string, string, string, bool) error { return nil }
	for _, src := range []string{"", "[x]\na = 1\n[x \"\"]\nb\n", "[x\n", "a = 1\n", "[x]\na = \"b\n"} {
		want := ReadWithCallback(strings.NewReader(src), nop)
		got := ReadWithOptions(strings.NewReader(src))
		if (got == nil) != (want == nil) || (got != nil && got.Error() != want.Error()) {
			t.Errorf("%q: got %v, want %v", src, got, want)
		}
	}
	if ReadWithOptions(strings.NewReader("[x\n")) == nil {
		t.Error("a syntax error must still be reported")
	}
}

func TestReadWithOptionsStopsOnCallbackError(t *testing.T) {
	stop := errors.New("stop")
	calls := 0
	err := ReadWithOptions(strings.NewReader("[x]\na = 1\nb = 2\n"),
		WithCallback(func(Event) error {
			calls++
			if calls == 2 {
				return stop
			}
			return nil
		}))
	if !errors.Is(err, stop) {
		t.Errorf("got %v, want %v", err, stop)
	}
	if calls != 2 {
		t.Errorf("callback called %d times, want 2", calls)
	}
}

func TestReadWithOptionsLastCallbackWins(t *testing.T) {
	var first, second int
	err := ReadWithOptions(strings.NewReader("[x]\n"),
		WithCallback(func(Event) error { first++; return nil }),
		WithCallback(func(Event) error { second++; return nil }))
	if err != nil {
		t.Fatal(err)
	}
	if first != 0 || second != 1 {
		t.Errorf("first=%d second=%d", first, second)
	}
}

func TestReadWithOptionsPosition(t *testing.T) {
	src := "; comment\n" + // 1
		"\n" + // 2
		"[a]\n" + // 3
		"\tk = 1\n" + // 4
		"  [b \"\"] # c\n" + // 5
		"\n" + // 6
		"\tl\n" + // 7
		"[c.d]\n" // 8
	type pos struct {
		key       string
		line, col int
	}
	want := []pos{
		{"", 3, 1}, {"k", 4, 2},
		{"", 5, 3}, {"l", 7, 2},
		{"", 8, 1},
	}
	var got []pos
	err := ReadWithOptions(strings.NewReader(src), WithCallback(func(e Event) error {
		got = append(got, pos{e.Key, e.Position.Line, e.Position.Column})
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

// Git reads a section name that starts with a digit or '-' in every
// spelling: bare, quoted subsection and legacy. Each text below gives the
// same variables as git config --list reports for it.
func TestReadSectionNameLeadingDigitOrHyphen(t *testing.T) {
	for _, tt := range []struct {
		text      string
		sect, sub string
		hasSub    bool
	}{
		{"[1a]", "1a", "", false},
		{"[-a]", "-a", "", false},
		{"[1a \"b\"]", "1a", "b", true},
		{"[-a \"B\"]", "-a", "B", true},
		{"[1a.b]", "1a", "b", true},
		{"[-a.b]", "-a", "b", true},
		{"[1.b]", "1", "b", true},
	} {
		t.Run(tt.text, func(t *testing.T) {
			var got []Event
			err := ReadWithOptions(strings.NewReader(tt.text+"\n\tk = v\n"),
				WithCallback(func(e Event) error { got = append(got, e); return nil }))
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 2 || got[0].Section != tt.sect || got[0].Subsection != tt.sub ||
				got[0].HasSubsection != tt.hasSub || got[1].Section != tt.sect ||
				got[1].Subsection != tt.sub || got[1].Key != "k" || got[1].Value != "v" {
				t.Errorf("want %q/%q (sub %v), got %+v", tt.sect, tt.sub, tt.hasSub, got)
			}
		})
	}
}

// The quoted form is what go-git's encoder writes for a legacy header, so
// it must read back as the same subsection.
func TestReadLegacyHeaderRoundTripsQuoted(t *testing.T) {
	for _, h := range []string{"1a.b", "-a.b", "a.b"} {
		sect, sub, _ := strings.Cut(h, ".")
		legacy, err := readCallbacks(t, "["+h+"]\n\tk = v\n")
		if err != nil {
			t.Fatal(err)
		}
		quoted, err := readCallbacks(t, "["+sect+" \""+sub+"\"]\n\tk = v\n")
		if err != nil {
			t.Fatalf("%s: quoted form: %v", h, err)
		}
		if !reflect.DeepEqual(legacy, quoted) {
			t.Errorf("%s: legacy %q, quoted %q", h, legacy, quoted)
		}
	}
}

// Positions cost a lookup per header and option, so only ReadWithOptions
// with a callback computes them.
func TestReadEventsPositions(t *testing.T) {
	const text = "[a]\n  k = v\n"
	for _, tt := range []struct {
		positions bool
		want      []token.Position
	}{
		{true, []token.Position{{Offset: 0, Line: 1, Column: 1}, {Offset: 6, Line: 2, Column: 3}}},
		{false, []token.Position{{}, {}}},
	} {
		var got []token.Position
		err := readEvents(strings.NewReader(text), func(e Event) error {
			got = append(got, e.Position)
			return nil
		}, tt.positions)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("positions=%v: want %+v, got %+v", tt.positions, tt.want, got)
		}
	}
}
