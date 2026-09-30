package formatter

import "testing"

func TestFormat(t *testing.T) {
	src := "package main\nimport \"fmt\"\n   class A {\n private x int   \n      public function F() {\n  switch this.x {\n    case 1:\n   fmt.Println(\"one\" +\n \"two\")\n      default:\n        s := `raw\n   keep   me`\n        _ = s\n  }\n  y := 1 +\n2\n  _ = y\n          }\n}\n\n\n\n// comment\nfunction main() {\nnew A().F()\n}"
	want := "package main\nimport \"fmt\"\nclass A {\n\tprivate x int\n\tpublic function F() {\n\t\tswitch this.x {\n\t\tcase 1:\n\t\t\tfmt.Println(\"one\" +\n\t\t\t\t\"two\")\n\t\tdefault:\n\t\t\ts := `raw\n   keep   me`\n\t\t\t_ = s\n\t\t}\n\t\ty := 1 +\n\t\t\t2\n\t\t_ = y\n\t}\n}\n\n// comment\nfunction main() {\n\tnew A().F()\n}\n"
	got, err := Format("t.gpp", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	again, _ := Format("t.gpp", got)
	if string(again) != string(got) {
		t.Fatal("formatting is not idempotent")
	}
}

func TestFormatRejectsInvalid(t *testing.T) {
	if _, err := Format("t.gpp", []byte("package main\nfunc {")); err == nil {
		t.Fatal("expected a syntax error")
	}
}
