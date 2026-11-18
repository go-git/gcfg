package gcfg_test

import (
	"fmt"
	"log"
	"strings"

	"github.com/go-git/gcfg/v2"
)

func ExampleReadStringInto() {
	cfgStr := `; Comment line
[section]
name=value # comment`
	cfg := struct {
		Section struct {
			Name string
		}
	}{}
	err := gcfg.ReadStringInto(&cfg, cfgStr)
	if err != nil {
		log.Fatalf("Failed to parse gcfg data: %s", err)
	}
	fmt.Println(cfg.Section.Name)
	// Output: value
}

func ExampleReadStringInto_bool() {
	cfgStr := `; Comment line
[section]
switch=on`
	cfg := struct {
		Section struct {
			Switch bool
		}
	}{}
	err := gcfg.ReadStringInto(&cfg, cfgStr)
	if err != nil {
		log.Fatalf("Failed to parse gcfg data: %s", err)
	}
	fmt.Println(cfg.Section.Switch)
	// Output: true
}

func ExampleReadStringInto_hyphens() {
	cfgStr := `; Comment line
[section-name]
variable-name=value # comment`
	cfg := struct {
		Section_Name struct {
			Variable_Name string
		}
	}{}
	err := gcfg.ReadStringInto(&cfg, cfgStr)
	if err != nil {
		log.Fatalf("Failed to parse gcfg data: %s", err)
	}
	fmt.Println(cfg.Section_Name.Variable_Name)
	// Output: value
}

func ExampleReadStringInto_tags() {
	cfgStr := `; Comment line
[section]
var-name=value # comment`
	cfg := struct {
		Section struct {
			FieldName string `gcfg:"var-name"`
		}
	}{}
	err := gcfg.ReadStringInto(&cfg, cfgStr)
	if err != nil {
		log.Fatalf("Failed to parse gcfg data: %s", err)
	}
	fmt.Println(cfg.Section.FieldName)
	// Output: value
}

func ExampleReadStringInto_subsections() {
	cfgStr := `; Comment line
[profile "A"]
color = white

[profile "B"]
color = black
`
	cfg := struct {
		Profile map[string]*struct {
			Color string
		}
	}{}
	err := gcfg.ReadStringInto(&cfg, cfgStr)
	if err != nil {
		log.Fatalf("Failed to parse gcfg data: %s", err)
	}
	fmt.Printf("%s %s\n", cfg.Profile["A"].Color, cfg.Profile["B"].Color)
	// Output: white black
}

func ExampleReadStringInto_multivalue() {
	cfgStr := `; Comment line
[section]
multi=value1
multi=value2`
	cfg := struct {
		Section struct {
			Multi []string
		}
	}{}
	err := gcfg.ReadStringInto(&cfg, cfgStr)
	if err != nil {
		log.Fatalf("Failed to parse gcfg data: %s", err)
	}
	fmt.Println(cfg.Section.Multi)
	// Output: [value1 value2]
}

func ExampleReadStringInto_unicode() {
	cfgStr := `; Comment line
[甲]
乙=丙 # comment`
	cfg := struct {
		X甲 struct {
			X乙 string
		}
	}{}
	err := gcfg.ReadStringInto(&cfg, cfgStr)
	if err != nil {
		log.Fatalf("Failed to parse gcfg data: %s", err)
	}
	fmt.Println(cfg.X甲.X乙)
	// Output: 丙
}

func ExampleReadWithOptions() {
	cfgStr := `[protocol]
allow = never
[protocol ""]
allow = always`
	err := gcfg.ReadWithOptions(strings.NewReader(cfgStr),
		gcfg.WithCallback(func(e gcfg.Event) error {
			if e.Key == "" {
				return nil
			}
			fmt.Printf("line %d: hasSubsection=%t %s=%s\n",
				e.Position.Line, e.HasSubsection, e.Key, e.Value)
			return nil
		}))
	if err != nil {
		log.Fatalf("Failed to parse gcfg data: %s", err)
	}
	// Output:
	// line 2: hasSubsection=false allow=never
	// line 4: hasSubsection=true allow=always
}

func ExampleReadWithOptions_validate() {
	// Without WithCallback the data is only parsed, so this checks syntax.
	err := gcfg.ReadWithOptions(strings.NewReader("[section\nname = value"))
	fmt.Println(err != nil)
	// Output: true
}
