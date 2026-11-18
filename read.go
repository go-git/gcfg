package gcfg

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/go-git/gcfg/v2/scanner"
	"github.com/go-git/gcfg/v2/token"
)

var unescape = map[rune]rune{'\\': '\\', '"': '"', 'n': '\n', 't': '\t', 'b': '\b'}

// no error: invalid literals should be caught by scanner
func unquote(s string) (string, error) {
	u, q, esc := make([]rune, 0, len(s)), false, false
	for _, c := range s {
		if esc {
			uc, ok := unescape[c]
			switch {
			case ok:
				u = append(u, uc)
				fallthrough
			case c == '\n':
				// A backslash at the end of a line continues the value
				// on the next one, and both are dropped, as git does.
				esc = false
				continue
			}
			return "", ErrMissingEscapeSequence
		}
		switch c {
		case '"':
			q = !q
		case '\\':
			esc = true
		default:
			u = append(u, c)
		}
	}
	if q {
		return "", ErrMissingEndQuote
	}
	if esc {
		return "", ErrMissingEscapeSequence
	}
	return string(u), nil
}

func read(callback func(string, string, bool, string, string, bool) error,
	fset *token.FileSet, file *token.File, src []byte) error {
	//
	var s scanner.Scanner
	var errs scanner.ErrorList
	s.Init(file, src, func(p token.Position, m string) { errs.Add(p, m) }, 0)
	sect, sectsub, hasSub := "", "", false
	pos, tok, lit, err := s.Scan()
	errfn := func(msg string) error {
		return fmt.Errorf("%s: %s", fset.Position(pos), msg)
	}
	if err != nil {
		return err
	}
	var accErr error
	for {
		if errs.Len() > 0 {
			if err, fatal := joinNonFatal(accErr, errs.Err()); fatal {
				return err
			}
		}
		switch tok {
		case token.EOF:
			return nil
		case token.EOL, token.COMMENT:
			pos, tok, lit, err = s.Scan()
			if err != nil {
				return err
			}
		case token.LBRACK:
			pos, tok, lit, err = s.Scan()
			if err != nil {
				return err
			}
			if errs.Len() > 0 {
				if err, fatal := joinNonFatal(accErr, errs.Err()); fatal {
					return err
				}
			}
			if tok != token.IDENT {
				if err, fatal := joinNonFatal(accErr, errfn("expected section name")); fatal {
					return err
				}
			}
			// The scanner only returns a dot in the name of a legacy
			// [section.subsection] header. Git reads it as the section up
			// to the first dot and the subsection after it, both
			// lowercased.
			sect, sectsub, hasSub = lit, "", false
			if tok == token.IDENT {
				if name, sub, ok := strings.Cut(lit, "."); ok {
					sect, sectsub, hasSub = strings.ToLower(name), strings.ToLower(sub), true
				}
			}
			pos, tok, lit, err = s.Scan()
			if err != nil {
				return err
			}
			if errs.Len() > 0 {
				if err, fatal := joinNonFatal(accErr, errs.Err()); fatal {
					return err
				}
			}
			if tok == token.STRING {
				ss, err := unquote(lit)
				if err != nil {
					return err
				}

				sectsub, hasSub = ss, true
				pos, tok, lit, err = s.Scan()
				if err != nil {
					return err
				}
				if errs.Len() > 0 {
					if err, fatal := joinNonFatal(accErr, errs.Err()); fatal {
						return err
					}
				}
			}
			if tok != token.RBRACK {
				if err, fatal := joinNonFatal(accErr, errfn("expected right bracket")); fatal {
					return err
				}
			}
			pos, tok, lit, err = s.Scan()
			if err != nil {
				return err
			}
			if tok != token.EOL && tok != token.EOF && tok != token.COMMENT {
				if err, fatal := joinNonFatal(accErr, errfn("expected EOL, EOF, or comment")); fatal {
					return err
				}
			}
			// If a section/subsection header was found, ensure a
			// container object is created, even if there are no
			// variables further down.
			err := callback(sect, sectsub, hasSub, "", "", true)
			if err != nil {
				return err
			}
		case token.IDENT:
			if sect == "" {
				if err, fatal := joinNonFatal(accErr, errfn("expected section header")); fatal {
					return err
				}
			}
			n := lit
			pos, tok, lit, err = s.Scan()
			if err != nil {
				return err
			}
			if errs.Len() > 0 {
				return errs.Err()
			}
			blank, v := tok == token.EOF || tok == token.EOL || tok == token.COMMENT, ""
			if !blank {
				if tok != token.ASSIGN {
					if err, fatal := joinNonFatal(accErr, errfn("expected '='")); fatal {
						return err
					}
				}
				pos, tok, lit, err = s.Scan()
				if err != nil {
					return err
				}
				if errs.Len() > 0 {
					if err, fatal := joinNonFatal(accErr, errs.Err()); fatal {
						return err
					}
				}
				if tok != token.STRING {
					if err, fatal := joinNonFatal(accErr, errfn("expected value")); fatal {
						return err
					}
				}
				unq, err := unquote(lit)
				if err != nil {
					return err
				}

				v = unq
				pos, tok, lit, err = s.Scan()
				if err != nil {
					return err
				}
				if errs.Len() > 0 {
					if err, fatal := joinNonFatal(accErr, errs.Err()); fatal {
						return err
					}
				}
				if tok != token.EOL && tok != token.EOF && tok != token.COMMENT {
					if err, fatal := joinNonFatal(accErr, errfn("expected EOL, EOF, or comment")); fatal {
						return err
					}
				}
			}
			err := callback(sect, sectsub, hasSub, n, v, blank)
			if err != nil {
				return err
			}
		default:
			if sect == "" {
				if err, fatal := joinNonFatal(accErr, errfn("expected section header")); fatal {
					return err
				}
			}
			if err, fatal := joinNonFatal(accErr, errfn("expected section header or variable declaration")); fatal {
				return err
			}
		}
	}
}

func readInto(config interface{}, fset *token.FileSet, file *token.File,
	src []byte) error {
	//
	firstPassCallback := func(s string, ss string, _ bool, k string, v string, bv bool) error {
		return set(config, s, ss, k, v, bv, false)
	}
	err := read(firstPassCallback, fset, file, src)
	if err != nil {
		return err
	}
	secondPassCallback := func(s string, ss string, _ bool, k string, v string, bv bool) error {
		return set(config, s, ss, k, v, bv, true)
	}
	return read(secondPassCallback, fset, file, src)
}

// ReadWithCallback reads gcfg formatted data from reader and calls
// callback with each section and option found.
//
// Callback is called with section, subsection, option key, option value
// and blank value flag as arguments.
//
// When a section is found, callback is called with an empty subsection,
// option key and option value.
//
// When a subsection is found, callback is called with an empty option key and
// option value.
//
// A header [x ""] is reported like [x], with the same empty subsection. A
// legacy [A.B] header is reported as section "a" and subsection "b".
// ReadWithSubsectionCallback tells [x] and [x ""] apart.
//
// If blank value flag is true, it means that the value was not set for an option
// (as opposed to set to empty string).
//
// If callback returns an error, ReadWithCallback terminates with an error too.
func ReadWithCallback(reader io.Reader, callback func(string, string, string, string, bool) error) error {
	return ReadWithSubsectionCallback(reader, func(s, ss string, _ bool, k, v string, bv bool) error {
		return callback(s, ss, k, v, bv)
	})
}

// ReadWithSubsectionCallback is like ReadWithCallback, but the callback also
// receives hasSubsection, which tells [x] from [x ""]. Both give an empty
// subsection, but Git reads them as x.key and x..key respectively.
//
// hasSubsection is true for a header with a quoted subsection, including an
// empty one, and for a legacy [section.subsection] header such as [a.b].
// It is false for a plain [x]. It describes the header being read, so it is
// also passed with the option lines that follow that header.
//
// As with ReadWithCallback, the callback is also called once per header, with
// an empty key and value and blankValue true.
//
// The Read*Into functions drop the flag, so an empty subsection there still
// folds onto the section.
func ReadWithSubsectionCallback(reader io.Reader, callback func(section, subsection string, hasSubsection bool, key, value string, blankValue bool) error) error {
	src, err := io.ReadAll(reader)
	if err != nil {
		return err
	}

	fset := token.NewFileSet()
	file, err := fset.AddFile("", fset.Base(), len(src))
	if err != nil {
		return err
	}

	return read(callback, fset, file, src)
}

// ReadInto reads gcfg formatted data from reader and sets the values into the
// corresponding fields in config.
//
// Git files the keys under [sec ""] as sec..key, which a struct target cannot
// represent, so ReadInto reads them as sec.key, like those under [sec]. A
// caller that applies a policy keyed on a subsection must use
// ReadWithSubsectionCallback instead.
func ReadInto(config interface{}, reader io.Reader) error {
	src, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	fset := token.NewFileSet()
	file, err := fset.AddFile("", fset.Base(), len(src))
	if err != nil {
		return err
	}
	return readInto(config, fset, file, src)
}

// ReadStringInto reads gcfg formatted data from str and sets the values into
// the corresponding fields in config.
//
// See ReadInto for how an empty subsection is read.
func ReadStringInto(config interface{}, str string) error {
	r := strings.NewReader(str)
	return ReadInto(config, r)
}

// ReadFileInto reads gcfg formatted data from the file filename and sets the
// values into the corresponding fields in config.
//
// See ReadInto for how an empty subsection is read.
func ReadFileInto(config interface{}, filename string) error {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	src, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	fset := token.NewFileSet()
	file, err := fset.AddFile(filename, fset.Base(), len(src))
	if err != nil {
		return err
	}
	return readInto(config, fset, file, src)
}
