package consts_test

//"github.com/alecthomas/repr"

// func TestINI(t *testing.T) {
// 	// Create a sample INI content
// 	iniContent := `
// [section1]
// key1 = value1
// key2 = value2

// [section2]
// key3 = value3
// `
// 	// Create a reader from the string
// 	reader := strings.NewReader(iniContent)

// 	// Parse the INI content
// 	p := consts.Parser()
// 	ini, err := p.Parse("", reader)
// 	if err != nil {
// 		t.Fatalf("Failed to parse INI: %v", err)
// 	}

// 	// Print the parsed INI for debugging
// 	t.Logf("Parsed INI: %s", repr.String(ini, repr.Indent("  "), repr.OmitEmpty(true)))

// 	// Add assertions to verify the parsed content
// 	if len(ini) != 2 {
// 		t.Errorf("Expected 2 sections, got %d", len(ini))
// 	}

// 	if val, ok := ini["section1"]["key1"]; !ok || val != "value1" {
// 		t.Errorf("Expected section1.key1 = value1, got %v", val)
// 	}

// 	if val, ok := ini["section2"]["key3"]; !ok || val != "value3" {
// 		t.Errorf("Expected section2.key3 = value3, got %v", val)
// 	}
// }

// func TestINIFromFile(t *testing.T) {
// 	// Create a temporary INI file
// 	tmpFile, err := os.CreateTemp("", "test-*.ini")
// 	if err != nil {
// 		t.Fatalf("Failed to create temp file: %v", err)
// 	}
// 	defer os.Remove(tmpFile.Name())

// 	// Write sample content to the file
// 	content := `
// [database]
// host = localhost
// port = 5432
// user = admin

// [app]
// debug = true
// log_level = info
// `
// 	if _, err := tmpFile.WriteString(content); err != nil {
// 		t.Fatalf("Failed to write to temp file: %v", err)
// 	}
// 	tmpFile.Close()

// 	// Open and parse the file
// 	file, err := os.Open(tmpFile.Name())
// 	if err != nil {
// 		t.Fatalf("Failed to open temp file: %v", err)
// 	}
// 	defer file.Close()

// 	ini, err := consts.ParseINI(tmpFile.Name(), file)
// 	if err != nil {
// 		t.Fatalf("Failed to parse INI file: %v", err)
// 	}

// 	// Verify parsed content
// 	if val, ok := ini["database"]["port"]; !ok || val != "5432" {
// 		t.Errorf("Expected database.port = 5432, got %v", val)
// 	}

// 	if val, ok := ini["app"]["debug"]; !ok || val != "true" {
// 		t.Errorf("Expected app.debug = true, got %v", val)
// 	}
// }
