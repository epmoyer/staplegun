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

const gTitle = "staplegun"
const gVersion = "1.0.0"

var gVerbose = false
var gDirSource = ""

var regexIsParent *regexp.Regexp
var regexIsChild *regexp.Regexp
var regexDefineBlock *regexp.Regexp
var regexEnd *regexp.Regexp
var regexInsertBlock *regexp.Regexp
var regexImportFile *regexp.Regexp

type blocksT map[string][]string

func init() {
	regexIsParent, _ = regexp.Compile(`^\s*{{\s*staplegun\s*parent\s*}}\s*$`)
	regexIsChild, _ = regexp.Compile(`^\s*{{\s*staplegun\s*child\s*}}\s*$`)
	regexDefineBlock, _ = regexp.Compile(`^\s*{{\s*staplegun\s*define_block\s*(\w+)\s*}}\s*$`)
	regexEnd, _ = regexp.Compile(`^\s*{{\s*staplegun\s*end\s*}}\s*$`)
	regexInsertBlock, _ = regexp.Compile(`^(\s*){{\s*staplegun\s*insert_block\s*(\w+)\s*}}\s*$`)
	regexImportFile, _ = regexp.Compile(`^(\s*){{\s*staplegun\s*import_file\s*(\S+)\s*}}\s*$`)
}

type VarMap map[string]string

func VersionInfo() string {
	return gTitle + " v" + gVersion
}

func MakeTemplates(dirSource string, dirDest string, verbose bool, varMap VarMap) error {
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
		blocksEmpty := blocksT{}
		isParent, isChild, linesOut, _, err := parseTemplate(sourceFile, blocksEmpty, indentLevel, varMap)
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

func parseTemplate(
	inFilePath string,
	blocksIn blocksT,
	indentLevel int,
	varMap VarMap) (
	isParent bool,
	isChild bool,
	linesOut []string,
	blocksOut blocksT,
	err error) {
	vPrintLn(indentLevel, fmt.Sprintf("Parsing %q", inFilePath))

	blocksOut = copyBlocks(blocksIn)

	// ------------------------
	// Read input file
	// ------------------------
	b, err := os.ReadFile(inFilePath)
	if err != nil {
		return false, false, []string{}, blocksT{}, err
	}
	text := string(b)
	lines := strings.Split(text, "\n")

	if len(lines) < 2 {
		// empty file
		return false, false, []string{}, blocksT{}, nil
	}

	isParent = regexIsParent.MatchString(lines[0])
	isChild = regexIsChild.MatchString(lines[0])
	if !isParent && !isChild {
		// Not a staplegun file, so ignore
		return false, false, []string{}, blocksT{}, nil
	}

	// Drop the first line (parent/child directive)
	lines = lines[1:]

	// ------------------------
	// PRE Parse insert_block directives
	// ------------------------
	vPrintLn(indentLevel+1, "Post-parsing insert_block directives...")

	// We ignore undefined blocks in the the pre-parsing pass.
	// The job of this pass is to resolve the content of any known blocks before extracting,
	// since there may be resolvable insert_block statements in the content of newly defined blocks,
	// and we want to resolve any "nested" block content before extracting the (new) blocks.
	ignoreUndefinedBlocks := true
	lines, err = parseInsertBlockDirectives(lines, blocksOut, ignoreUndefinedBlocks, indentLevel)
	if err != nil {
		return false, false, []string{}, blocksT{}, err
	}

	// ------------------------
	// Extract Blocks
	// ------------------------
	var blocksExtracted blocksT
	blocksExtracted, lines, err = extractBlocks(lines, indentLevel)
	if err != nil {
		return false, false, []string{}, blocksT{}, err
	}
	vPrintLn(indentLevel+1, fmt.Sprintf("Extracted %d blocks.", len(blocksExtracted)))
	blocksOut = mergeBlocks(blocksOut, blocksExtracted)
	// vPrintLn(fmt.Sprintf("        Blocks: %#v", blocks))

	// ------------------------
	// Parse import_file directives
	// ------------------------
	var blocksNew blocksT
	lines, blocksNew, err = parseImportFileDirectives(lines, blocksOut, indentLevel, varMap)
	if err != nil {
		return false, false, []string{}, blocksOut, err
	}
	if len(blocksNew) > len(blocksOut) {
		vPrintLn(
			indentLevel+1,
			fmt.Sprintf("Received %d new blocks from all imports combined.", len(blocksNew)-len(blocksOut)))
		blocksOut = copyBlocks(blocksNew)
	}

	// ------------------------
	// POST Parse insert_block directives
	// ------------------------
	vPrintLn(indentLevel+1, "Post-parsing insert_block directives...")

	// Blocks are generally not defined yet when a child document is parsed, so
	// if an insert_block statement in a child document references an undefined block
	// then we ignore the warning and retain the inset_block statement, which will
	// then be resolved at a higher level in the import_file hierarchy (or eventually
	// cause an error to reported in the parent document).
	ignoreUndefinedBlocks = isChild
	lines, err = parseInsertBlockDirectives(lines, blocksOut, ignoreUndefinedBlocks, indentLevel)
	if err != nil {
		return false, false, []string{}, blocksT{}, err
	}

	// ------------------------
	// Substitute variables
	// ------------------------
	lines = substituteVariables(lines, indentLevel, varMap)

	// ------------------------
	// Return parsed document
	// ------------------------
	return isParent, isChild, lines, blocksOut, nil
}

func substituteVariables(linesIn []string, indentLevel int, varMap VarMap) (linesOut []string) {
	for _, line := range linesIn {
		for varName, varValue := range varMap {
			// Create a regex pattern with zero or more whitespaces in appropriate places and one or more whitespace before/after var
			pattern := regexp.MustCompile(`{{\s*staplegun\s+var\s+` + regexp.QuoteMeta(varName) + `\s*}}`)
			line = pattern.ReplaceAllString(line, varValue)
		}
		linesOut = append(linesOut, line)
	}
	return linesOut
}

func parseInsertBlockDirectives(linesIn []string,
	blocks blocksT,
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
			vPrintLn(indentLevel+1, fmt.Sprintf("RESOLVED insert_block: %q", blockName))
			continue
		}

		// Content line
		linesOut = append(linesOut, line)
	}
	return linesOut, nil
}

func parseImportFileDirectives(linesIn []string,
	blocksIn blocksT,
	indentLevel int,
	varMap VarMap) (
	linesOut []string,
	blocksOut blocksT,
	err error) {

	blocksOut = copyBlocks(blocksIn)

	for _, line := range linesIn {

		// -------------------
		// Import file
		// -------------------
		match := regexImportFile.FindStringSubmatch(line)
		if len(match) == 3 {
			indentWhitespace := match[1]
			filename := match[2]

			importFilePath := gDirSource + "/" + filename
			vPrintLn(indentLevel+1, fmt.Sprintf("Importing %q", importFilePath))

			var importedLines []string
			var blocksExtracted blocksT

			_, _, importedLines, blocksExtracted, err = parseTemplate(importFilePath, blocksOut, indentLevel+2, varMap)
			if err != nil {
				return []string{}, blocksT{}, err
			}
			blocksOut = mergeBlocks(blocksOut, blocksExtracted)
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
	return linesOut, blocksOut, nil
}

func copyBlocks(blocksIn blocksT) (blocksOut blocksT) {
	blocksOut = make(blocksT)
	for blockName, blockLines := range blocksIn {
		blocksOut[blockName] = blockLines
	}
	return blocksOut
}

func mergeBlocks(blocks1 blocksT, blocks2 blocksT) (blocksOut blocksT) {
	blocksOut = copyBlocks(blocks1)
	for blockName, blockLines := range blocks2 {
		blocksOut[blockName] = copyLines(blockLines)
	}
	return blocksOut
}

func copyLines(linesIn []string) (linesOut []string) {
	linesOut = make([]string, len(linesIn))
	copy(linesOut, linesIn)
	return linesOut
}

func extractBlocks(linesIn []string, indentLevel int) (blocksOut blocksT, linesOut []string, err error) {
	blocksOut = make(blocksT)
	currentBlockName := ""
	currentBlockLines := []string{}
	for _, line := range linesIn {

		// Start of define block
		match := regexDefineBlock.FindStringSubmatch(line)
		// vPrintLn(fmt.Sprintf("match: %#v", match))
		if len(match) == 2 {
			if currentBlockName != "" {
				return make(blocksT),
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
				return make(blocksT), []string{}, fmt.Errorf("found end of block when no block was started")
			}
			blocksOut[currentBlockName] = currentBlockLines
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
		return make(blocksT),
			[]string{},
			fmt.Errorf(
				"block %q was not closed",
				currentBlockName)
	}

	return blocksOut, linesOut, nil
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
