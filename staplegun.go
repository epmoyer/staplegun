// TODO:
// - Error gracefully on circular import somehow
//

package staplegun

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var gVerbose = false
var gDirSource = ""

var regexIsParent *regexp.Regexp
var regexIsChild *regexp.Regexp
var regexDefineBlock *regexp.Regexp
var regexEnd *regexp.Regexp
var regexInsertBlock *regexp.Regexp
var regexImportFile *regexp.Regexp

func init() {
	regexIsParent, _ = regexp.Compile(`^\s*{{\s*staplegun\s*parent\s*}}\s*$`)
	regexIsChild, _ = regexp.Compile(`^\s*{{\s*staplegun\s*child\s*}}\s*$`)
	regexDefineBlock, _ = regexp.Compile(`^\s*{{\s*staplegun\s*define_block\s*(\w+)\s*}}\s*$`)
	regexEnd, _ = regexp.Compile(`^\s*{{\s*staplegun\s*end\s*}}\s*$`)
	regexInsertBlock, _ = regexp.Compile(`^(\s*){{\s*staplegun\s*insert_block\s*(\w+)\s*}}\s*$`)
	regexImportFile, _ = regexp.Compile(`^(\s*){{\s*staplegun\s*import_file\s*(\S+)\s*}}\s*$`)
}

func MakeTemplates(dirSource string, dirDest string, verbose bool) error {
	gVerbose = verbose
	gDirSource = dirSource

	vPrintLn(0, "staplegun:MakeTemplates():")

	// ------------------------
	// Validate arguments
	// ------------------------
	isDir, _ := isDirectory(dirSource)
	if !isDir {
		return fmt.Errorf("%#v is not a directory", dirSource)
	}
	isDir, _ = isDirectory(dirDest)
	if !isDir {
		return fmt.Errorf("%#v is not a directory", dirSource)
	}

	// ------------------------
	// Parse source files
	// ------------------------
	sourceFiles, err := filepath.Glob(dirSource + "/*")
	if err != nil {
		return err
	}
	for _, sourceFile := range sourceFiles {
		// Ignore directories
		isDir, _ := isDirectory(sourceFile)
		if isDir {
			continue
		}

		// Pase the template
		filenameBase := filepath.Base(sourceFile)
		indentLevel := 1
		isParent, isChild, linesOut, err := parseTemplate(sourceFile, indentLevel)
		if err != nil {
			return err
		}

		// Ignore Child docs and non-staplegun docs
		if isChild {
			vPrintLn(2, fmt.Sprintf("IGNORED %q because it is a child document.", filenameBase))
			continue
		}
		if !isParent && !isChild {
			vPrintLn(2, fmt.Sprintf("IGNORED %q because it is not a staplegun document.", filenameBase))
			continue
		}

		// Write out parsed document.
		if isParent {
			outFilePath := dirDest + "/" + filepath.Base(sourceFile)
			textOut := strings.Join(linesOut, "\n")
			err := os.WriteFile(outFilePath, []byte(textOut), 0644)
			if err != nil {
				return err
			}
			vPrintLn(2, fmt.Sprintf("WROTE parsed  %q -> %q", filenameBase, outFilePath))
		}
	}
	return nil
}

func parseTemplate(inFilePath string, indentLevel int) (isParent bool, isChild bool, linesOut []string, err error) {
	vPrintLn(indentLevel, fmt.Sprintf("Parsing %q", inFilePath))

	// ------------------------
	// Read input file
	// ------------------------
	b, err := os.ReadFile(inFilePath)
	if err != nil {
		return false, false, []string{}, err
	}
	text := string(b)
	lines := strings.Split(text, "\n")

	if len(lines) < 2 {
		// empty file
		return false, false, []string{}, nil
	}

	isParent = regexIsParent.MatchString(lines[0])
	isChild = regexIsChild.MatchString(lines[0])
	if !isParent && !isChild {
		// Not a staplegun file, so ignore
		return false, false, []string{}, nil
	}

	// Drop the first line (parent/child directive)
	lines = lines[1:]

	// ------------------------
	// Extract Blocks
	// ------------------------
	blocks, lines, err := extractBlocks(lines, indentLevel)
	if err != nil {
		return false, false, []string{}, err
	}
	vPrintLn(indentLevel+1, fmt.Sprintf("Extracted %d blocks.", len(blocks)))
	// vPrintLn(fmt.Sprintf("        Blocks: %#v", blocks))

	// ------------------------
	// Parse import_file directives
	// ------------------------
	lines, err = parseImportFileDirectives(lines, blocks, indentLevel)
	if err != nil {
		return false, false, []string{}, err
	}

	// ------------------------
	// Parse insert_block directives
	// ------------------------

	// Blocks are generally not defined yet when a child document is parsed, so
	// if an insert_block statement in a child document references an undefined block
	// then we ignore the warning and retain the inset_block statement, which will
	// then be resolved at a higher level in the import_file hierarchy (or eventually
	// cause an error to reported in the parent document).
	ignoreUndefinedBlocks := isChild
	lines, err = parseInsertBlockDirectives(lines, blocks, ignoreUndefinedBlocks, indentLevel)
	if err != nil {
		return false, false, []string{}, err
	}

	// ------------------------
	// Return parsed document
	// ------------------------
	return isParent, isChild, lines, nil
}

func parseInsertBlockDirectives(linesIn []string,
	blocks map[string][]string,
	ignoreUndefinedBlocks bool,
	indentLevel int) (linesOut []string, err error) {
	for _, line := range linesIn {

		// -------------------
		// Insert Block
		// -------------------
		match := regexInsertBlock.FindStringSubmatch(line)
		if len(match) == 3 {
			indentWhitespace := match[1]
			blockName := match[2]
			blockLines, ok := blocks[blockName]
			if !ok {
				// Block not defined
				if ignoreUndefinedBlocks {
					// Retain the inert_block statement (it will be resolved
					// by some parent document)
					linesOut = append(linesOut, line)
					vPrintLn(indentLevel+1, fmt.Sprintf("Retained insert_block of %q for resolution by some parent", blockName))
					continue
				}
				return []string{},
					fmt.Errorf(
						"insert_block target %q not defined", blockName)
			}
			// Insert the block, prepending each block line with the same indentation as
			// the insert_block statement
			linesOut = append(linesOut, indentWhitespace+"<!-- sg:block:start:"+blockName+" -->")
			for _, blockLine := range blockLines {
				linesOut = append(linesOut, indentWhitespace+blockLine)
			}
			linesOut = append(linesOut, indentWhitespace+"<!-- sg:block:end:"+blockName+" -->")
			continue
		}

		// Content line
		linesOut = append(linesOut, line)
	}
	return linesOut, nil
}

func parseImportFileDirectives(linesIn []string,
	blocks map[string][]string,
	indentLevel int) (linesOut []string, err error) {
	for _, line := range linesIn {

		// -------------------
		// Import file
		// -------------------
		match := regexImportFile.FindStringSubmatch(line)
		if len(match) == 3 {
			indentWhitespace := match[1]
			filename := match[2]
			_, _, importedLines, err := parseTemplate(gDirSource+"/"+filename, indentLevel+1)
			if err != nil {
				return []string{}, err
			}
			// Insert the imported lines, prepending each line with the same indentation as
			// the insert_block statement
			linesOut = append(linesOut, indentWhitespace+"<!-- sg:file:start:"+filename+" -->")
			for _, line = range importedLines {
				linesOut = append(linesOut, indentWhitespace+line)
			}
			linesOut = append(linesOut, indentWhitespace+"<!-- sg:file:end:"+filename+" -->")
			continue
		}

		// Content line
		linesOut = append(linesOut, line)
	}
	return linesOut, nil
}

func extractBlocks(linesIn []string, indentLevel int) (blocks map[string][]string, linesOut []string, err error) {
	blocks = make(map[string][]string)
	currentBlockName := ""
	currentBlockLines := []string{}
	for _, line := range linesIn {

		// Start of define block
		match := regexDefineBlock.FindStringSubmatch(line)
		// vPrintLn(fmt.Sprintf("match: %#v", match))
		if len(match) == 2 {
			if currentBlockName != "" {
				return make(map[string][]string),
					[]string{},
					fmt.Errorf(
						"found start of block when a previous block %q was not closed",
						currentBlockName)
			}
			currentBlockName = match[1]
			currentBlockLines = []string{}
			continue
		}

		// End of define block
		if regexEnd.MatchString(line) {
			if currentBlockName == "" {
				return make(map[string][]string), []string{}, fmt.Errorf("found end of block when no block was started")
			}
			blocks[currentBlockName] = currentBlockLines
			currentBlockName = ""
			currentBlockLines = []string{}
			continue
		}

		// Capturing a block
		if currentBlockName != "" {
			currentBlockLines = append(currentBlockLines, line)
			continue
		}

		// Content line
		linesOut = append(linesOut, line)
	}

	// Unclosed block?
	if currentBlockName != "" {
		return make(map[string][]string),
			[]string{},
			fmt.Errorf(
				"block %q was not closed",
				currentBlockName)
	}

	return blocks, linesOut, nil
}

// Verbose print
func vPrintLn(indentLevel int, text string) {
	if gVerbose {
		fmt.Println(strings.Repeat("    ", indentLevel) + text)
	}
}

// Return true if path is a directory
func isDirectory(path string) (bool, error) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return false, err
	}

	return fileInfo.IsDir(), err
}
