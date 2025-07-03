package tag_test

import (
	"fmt"
	"log"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

var gTest *testing.T

func TestGolden(t *testing.T) {
	err := os.Chdir("./")
	if err != nil {
		log.Fatal(err)
	}

	scriptDir := "golden/"
	files, err := os.ReadDir(scriptDir)
	if err != nil {
		log.Fatal(err)
	}
	const suffix = ".out.txt"
	for _, fi := range files {
		itemName := fi.Name()
		if itemName == "" || fi.IsDir() || itemName[0] == '.' {
			continue
		}

		if strings.HasSuffix(itemName, suffix) {
			continue
		}
		if !strings.HasSuffix(itemName, ".txt") {
			continue
		}

		outName := itemName[:len(itemName)-4] + suffix
		processTags(
			path.Join(scriptDir, itemName),
			path.Join(scriptDir, outName),
		)
	}
}

func echoLine(out *strings.Builder, line string) tag.UID {
	billet := tag.Name{}.With(line)
	if billet.ID.IsSet() {
		fmt.Fprintf(out, "%28s   ", "")
		out.WriteString(line)
		out.WriteByte('\n')
		fmt.Fprintf(out, "%28s   %s", billet.ID.Base32(), billet.Canonic)
	}
	out.WriteByte('\n')
	return billet.ID
}

func processTags(pathIn, pathOut string) {
	fileOut, err := os.Create(pathOut)
	if err != nil {
		gTest.Fatal(err)
	}
	defer fileOut.Close()
	fileIn, err := os.ReadFile(pathIn)
	if err != nil {
		gTest.Fatal(err)
	}

	b := strings.Builder{}

	{
		echoLine(&b, tag.PackageTags+"  . ✙ בְּרֵאשִׁ֖ית בָּרָ֣א אֱלֹהִ֑ים אֵ֥ת הַשָּׁמַ֖יִם וְאֵ֥ת הָאָֽרֶץ  ✙ .")
		fileOut.Write([]byte(b.String()))
	}

	for _, line := range strings.Split(string(fileIn), "\n") {
		b.Reset()
		echoLine(&b, line)
		if _, err := fileOut.Write([]byte(b.String())); err != nil {
			gTest.Fatal(err)
		}
	}
}
