package tui

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// The output pump owns byte order. These three entry points serialize parser,
// resize and snapshot access; a snapshot owns its values after the lock returns.
// On any feed error the transport must fail the case, never assert a valid frame.
type bddNativeFrameDecoder struct {
	mu                    sync.Mutex
	parser                *ansi.Parser
	width, height         int
	primary, alternate    bddNativeFrameBuffer
	inAlternate           bool
	overflow              []bddNativeFrameOverflow
	fullClearSequence     uint64
	err                   error
	sequenceBytes, digits int
	parameterSeparators   int
	oscCommandPrefix      bool
	oscCommandDigits      int
}

type bddNativeFrameCell struct {
	text  string
	width int // -1 is the second cell of a wide glyph; zero is a blank cell.
}

type bddNativeFrameBuffer struct {
	cells                       [][]bddNativeFrameCell
	row, column                 int
	intendedRow, intendedColumn int
	lastRow, lastColumn         int
	hasLast, wrapPending        bool
	autoWrap                    bool
	savedRow, savedColumn       int
	scrollTop, scrollBottom     int
	repeatRune                  rune
	canRepeat                   bool
	currentOverflow             []bddNativeFrameOverflow
	fullClearSequence           uint64
}

func newBDDNativeFrameDecoder(width, height int) *bddNativeFrameDecoder {
	screen := &bddNativeFrameDecoder{width: width, height: height}
	if !bddNativeFrameSizeSupported(width, height) {
		screen.err = errors.New("unsupported frame viewport size")
		return screen
	}
	screen.primary = newBDDNativeFrameBuffer(width, height)
	screen.alternate = newBDDNativeFrameBuffer(width, height)
	screen.parser = ansi.NewParser()
	screen.parser.SetDataSize(4096)
	screen.parser.SetHandler(ansi.Handler{
		Print: screen.printRune, Execute: screen.executeControl,
		HandleCsi: screen.executeCSI, HandleEsc: screen.executeESC,
		HandleOsc: screen.executeOSC,
		HandleDcs: func(_ ansi.Cmd, _ ansi.Params, _ []byte) { screen.reject("DCS payload") },
		HandleApc: func(_ []byte) { screen.reject("APC payload") },
		HandlePm:  func(_ []byte) { screen.reject("PM payload") },
		HandleSos: func(_ []byte) { screen.reject("SOS payload") },
	})
	return screen
}

func bddNativeFrameSizeSupported(width, height int) bool {
	return width > 0 && width <= 1024 && height > 0 && height <= 256
}

func newBDDNativeFrameBuffer(width, height int) bddNativeFrameBuffer {
	rows := make([][]bddNativeFrameCell, height)
	for row := range rows {
		rows[row] = make([]bddNativeFrameCell, width)
	}
	return bddNativeFrameBuffer{cells: rows, autoWrap: true, scrollBottom: height - 1}
}

func (screen *bddNativeFrameDecoder) active() *bddNativeFrameBuffer {
	if screen.inAlternate {
		return &screen.alternate
	}
	return &screen.primary
}

func (screen *bddNativeFrameDecoder) reject(operation string) {
	if screen.err == nil {
		screen.err = fmt.Errorf("unsupported terminal frame operation: %s", operation)
	}
}

func (screen *bddNativeFrameDecoder) feed(chunk []byte) error {
	screen.mu.Lock()
	defer screen.mu.Unlock()
	if screen.err != nil {
		return screen.err
	}
	for _, value := range chunk {
		if err := screen.feedByte(value); err != nil {
			return err
		}
	}
	return nil
}

func (screen *bddNativeFrameDecoder) feedByte(value byte) error {
	if screen.sequenceBytes >= 4096 {
		screen.reject("terminal sequence exceeds parser bounds")
		return screen.err
	}
	state := screen.parser.StateName()
	if !screen.acceptOSCCommandByte(state, value) {
		return screen.err
	}
	if state == "GroundState" && (value >= 0xa0 && value <= 0xc1 || value > 0xf4) {
		screen.reject("invalid UTF-8 byte")
		return screen.err
	}
	if strings.HasPrefix(state, "Csi") {
		if value >= '0' && value <= '9' {
			screen.digits++
		} else {
			screen.digits = 0
		}
		if value == ';' || value == ':' {
			screen.parameterSeparators++
		}
		if screen.digits > 5 || screen.parameterSeparators >= 31 {
			screen.reject("CSI parameter exceeds parser bounds")
			return screen.err
		}
	}
	screen.parser.Advance(value)
	if screen.err != nil {
		return screen.err
	}
	if screen.parser.StateName() == "OscStringState" && state != "OscStringState" {
		screen.oscCommandPrefix, screen.oscCommandDigits = true, 0
	}
	if screen.parser.StateName() == "GroundState" {
		screen.sequenceBytes, screen.digits, screen.parameterSeparators = 0, 0, 0
		screen.oscCommandPrefix, screen.oscCommandDigits = false, 0
	} else {
		screen.sequenceBytes++
		if screen.sequenceBytes > 4096 {
			screen.reject("terminal sequence exceeds parser bounds")
			return screen.err
		}
	}
	return nil
}

// Bound only the decimal command before x/ansi converts it to int. Its OSC
// payload is opaque and may contain arbitrarily long numbers within the existing
// sequence bound. Parser state transitions retain this distinction across chunks.
func (screen *bddNativeFrameDecoder) acceptOSCCommandByte(state string, value byte) bool {
	if state != "OscStringState" || !screen.oscCommandPrefix {
		return true
	}
	switch {
	case value >= '0' && value <= '9':
		screen.oscCommandDigits++
		if screen.oscCommandDigits > 5 {
			screen.reject("OSC command exceeds numeric parser bounds")
			return false
		}
	case value == ';' || value == 7 || value == 0x1b || value == 0x9c:
		if screen.oscCommandDigits == 0 {
			screen.reject("empty OSC command prefix")
			return false
		}
		screen.oscCommandPrefix = false
	default:
		screen.reject("nondecimal OSC command prefix")
		return false
	}
	return true
}

// finish is called at output EOF before a transport claims it joined. A partial
// sequence is legitimate between chunks, but cannot become a completed capture.
func (screen *bddNativeFrameDecoder) finish() error {
	screen.mu.Lock()
	defer screen.mu.Unlock()
	if screen.err != nil {
		return screen.err
	}
	if screen.parser.StateName() != "GroundState" {
		screen.reject("unterminated sequence at output EOF")
	}
	return screen.err
}

func (screen *bddNativeFrameDecoder) resize(width, height int) {
	screen.mu.Lock()
	defer screen.mu.Unlock()
	if screen.err != nil {
		return
	}
	if !bddNativeFrameSizeSupported(width, height) {
		screen.reject("viewport resize")
		return
	}
	screen.resizeBuffer(&screen.primary, width, height)
	screen.resizeBuffer(&screen.alternate, width, height)
	screen.width, screen.height = width, height
}

func (screen *bddNativeFrameDecoder) resizeBuffer(buffer *bddNativeFrameBuffer, width, height int) {
	resized := newBDDNativeFrameBuffer(width, height)
	for row := range min(height, screen.height) {
		copy(resized.cells[row], buffer.cells[row][:min(width, screen.width)])
		if width < screen.width && resized.cells[row][width-1].width == 2 {
			resized.cells[row][width-1] = bddNativeFrameCell{}
		}
	}
	// Resize cannot erase evidence or fabricate a producer repaint.
	resized.currentOverflow, resized.fullClearSequence = buffer.currentOverflow, buffer.fullClearSequence
	resized.autoWrap = buffer.autoWrap
	resized.repeatRune, resized.canRepeat = buffer.repeatRune, buffer.canRepeat
	resized.row, resized.column = min(buffer.row, height-1), min(buffer.column, width-1)
	resized.intendedRow, resized.intendedColumn = resized.row, resized.column
	resized.savedRow, resized.savedColumn = min(buffer.savedRow, height-1), min(buffer.savedColumn, width-1)
	*buffer = resized
}

func (screen *bddNativeFrameDecoder) snapshot() bddNativeFrameSnapshot {
	screen.mu.Lock()
	defer screen.mu.Unlock()
	buffer := screen.active()
	rows := make([]string, len(buffer.cells))
	for row, cells := range buffer.cells {
		var line strings.Builder
		for _, cell := range cells {
			switch cell.width {
			case -1:
			case 0:
				line.WriteByte(' ')
			default:
				line.WriteString(cell.text)
			}
		}
		rows[row] = strings.TrimRight(line.String(), " ")
	}
	return bddNativeFrameSnapshot{
		Rows: rows, Width: screen.width, Height: screen.height,
		CursorRow: buffer.row, CursorColumn: buffer.column, Alternate: screen.inAlternate,
		Overflow:        slices.Clone(screen.overflow),
		CurrentOverflow: slices.Clone(buffer.currentOverflow), FullClearSequence: buffer.fullClearSequence,
	}
}

func (screen *bddNativeFrameDecoder) printRune(character rune) {
	if screen.err != nil {
		return
	}
	if character == utf8.RuneError || character == '\u200d' || character >= '\U0001f1e6' && character <= '\U0001f1ff' || character >= '\U0001f3fb' && character <= '\U0001f3ff' {
		screen.reject("unsupported complex grapheme or invalid UTF-8")
		return
	}
	buffer := screen.active()
	text := string(character)
	cells := ansi.StringWidth(text)
	if unicode.Is(unicode.Mn, character) || unicode.Is(unicode.Me, character) {
		screen.combineRune(buffer, text)
		return
	}
	if cells == 0 {
		screen.reject("unsupported zero-width printable rune")
		return
	}
	if cells < 1 || cells > 2 || cells > screen.width {
		screen.reject("unsupported printable cell width")
		return
	}
	screen.recordOverflow(buffer, text, cells)
	if screen.err != nil {
		return
	}
	if buffer.wrapPending || buffer.column+cells > screen.width {
		if !buffer.autoWrap {
			if cells != 1 {
				screen.reject("wide glyph with autowrap disabled")
				return
			}
		} else {
			buffer.column = 0
			screen.lineFeed(buffer)
		}
	}
	for column := buffer.column; column < buffer.column+cells; column++ {
		screen.clearCell(buffer, buffer.row, column)
	}
	buffer.cells[buffer.row][buffer.column] = bddNativeFrameCell{text: text, width: cells}
	if cells == 2 {
		buffer.cells[buffer.row][buffer.column+1] = bddNativeFrameCell{width: -1}
	}
	buffer.lastRow, buffer.lastColumn, buffer.hasLast = buffer.row, buffer.column, true
	buffer.repeatRune, buffer.canRepeat = character, character >= 0x20 && character <= 0x7e
	end := buffer.column + cells
	buffer.column, buffer.wrapPending = min(end, screen.width-1), end == screen.width && buffer.autoWrap
	buffer.intendedRow, buffer.intendedColumn = buffer.row, end
	if !buffer.autoWrap {
		buffer.intendedColumn = buffer.column
	}
}

func (screen *bddNativeFrameDecoder) combineRune(buffer *bddNativeFrameBuffer, suffix string) {
	if !buffer.hasLast {
		screen.reject("combining rune without a preceding glyph")
		return
	}
	cell := &buffer.cells[buffer.lastRow][buffer.lastColumn]
	combined := cell.text + suffix
	if len(combined) > 256 || ansi.StringWidth(combined) != cell.width {
		screen.reject("combining sequence changes occupied cells")
		return
	}
	cell.text = combined
	buffer.canRepeat = false // Current renderer emits REP for ASCII graphics only.
}

func (screen *bddNativeFrameDecoder) recordOverflow(buffer *bddNativeFrameBuffer, text string, cells int) {
	row, column := buffer.intendedRow, buffer.intendedColumn
	if row >= 0 && row < screen.height && column >= 0 && column+cells <= screen.width {
		return
	}
	if len(screen.overflow) >= 4096 {
		screen.reject("overflow evidence exceeds bounded capture")
		return
	}
	write := bddNativeFrameOverflow{Row: row, Column: column, Cells: cells, Text: text, Width: screen.width, Height: screen.height}
	screen.overflow = append(screen.overflow, write)
	buffer.currentOverflow = append(buffer.currentOverflow, write)
}

func (screen *bddNativeFrameDecoder) clearCell(buffer *bddNativeFrameBuffer, row, column int) {
	cell := buffer.cells[row][column]
	if cell.width == -1 && column > 0 {
		buffer.cells[row][column-1] = bddNativeFrameCell{}
	}
	if cell.width == 2 && column+1 < screen.width {
		buffer.cells[row][column+1] = bddNativeFrameCell{}
	}
	buffer.cells[row][column] = bddNativeFrameCell{}
}

func (screen *bddNativeFrameDecoder) move(buffer *bddNativeFrameBuffer, row, column int) {
	buffer.intendedRow, buffer.intendedColumn = row, column
	buffer.row, buffer.column = max(0, min(row, screen.height-1)), max(0, min(column, screen.width-1))
	buffer.wrapPending, buffer.hasLast = false, false
}

func (screen *bddNativeFrameDecoder) lineFeed(buffer *bddNativeFrameBuffer) {
	buffer.intendedRow, buffer.intendedColumn = buffer.row+1, buffer.column
	if buffer.row == buffer.scrollBottom {
		screen.shiftLines(buffer, buffer.scrollTop, buffer.scrollBottom, 1, false)
	} else if buffer.row < screen.height-1 {
		buffer.row++
	}
	buffer.wrapPending, buffer.hasLast = false, false
}

// Move complete rows, preserving glyph pairs and rows outside the scroll region.
func (screen *bddNativeFrameDecoder) shiftLines(buffer *bddNativeFrameBuffer, top, bottom, count int, down bool) {
	count = min(count, bottom-top+1)
	if down {
		copy(buffer.cells[top+count:bottom+1], buffer.cells[top:bottom+1-count])
		for row := top; row < top+count; row++ {
			buffer.cells[row] = make([]bddNativeFrameCell, screen.width)
		}
	} else {
		copy(buffer.cells[top:bottom+1-count], buffer.cells[top+count:bottom+1])
		for row := bottom + 1 - count; row <= bottom; row++ {
			buffer.cells[row] = make([]bddNativeFrameCell, screen.width)
		}
	}
	buffer.wrapPending, buffer.hasLast = false, false
}

func (screen *bddNativeFrameDecoder) spliceCells(buffer *bddNativeFrameBuffer, count int, insert bool) {
	row, column := buffer.cells[buffer.row], buffer.column
	count = min(count, screen.width-column)
	if insert {
		copy(row[column+count:], row[column:screen.width-count])
		clear(row[column : column+count])
	} else {
		copy(row[column:], row[column+count:])
		clear(row[screen.width-count:])
	}
	// Remove only glyph halves split by the splice or clipped at the right edge.
	for index, cell := range row {
		if cell.width == -1 && (index == 0 || row[index-1].width != 2) ||
			cell.width == 2 && (index+1 == len(row) || row[index+1].width != -1) {
			row[index] = bddNativeFrameCell{}
		}
	}
	buffer.wrapPending, buffer.hasLast = false, false
}

func (screen *bddNativeFrameDecoder) setScrollRegion(buffer *bddNativeFrameBuffer, params ansi.Params) {
	top := bddNativeFrameParameter(params, 0, 1)
	bottom := bddNativeFrameParameter(params, 1, screen.height)
	if top == 0 {
		top = 1
	}
	if bottom == 0 {
		bottom = screen.height
	}
	if top < 1 || bottom > screen.height || top >= bottom {
		screen.reject("unsupported scroll region bounds")
		return
	}
	buffer.scrollTop, buffer.scrollBottom = top-1, bottom-1
	screen.move(buffer, 0, 0) // DECSTBM homes the cursor with origin mode disabled.
}

func (screen *bddNativeFrameDecoder) rendererOperation(command byte, params ansi.Params) bool {
	switch command {
	case 'L', 'M', 'S', 'T', '@', 'P', 'b', 'Z', 'r':
	default:
		return false
	}
	limit := 1
	if command == 'r' {
		limit = 2
	}
	if len(params) > limit {
		screen.reject("extra parameters in renderer operation")
		return true
	}
	buffer := screen.active()
	count := max(1, bddNativeFrameParameter(params, 0, 1))
	switch command {
	case 'L', 'M':
		if buffer.row >= buffer.scrollTop && buffer.row <= buffer.scrollBottom {
			screen.shiftLines(buffer, buffer.row, buffer.scrollBottom, count, command == 'L')
			screen.move(buffer, buffer.row, 0) // xterm IL/DL resets the cursor to the left margin.
		}
	case 'S', 'T':
		screen.shiftLines(buffer, buffer.scrollTop, buffer.scrollBottom, count, command == 'T')
	case '@', 'P':
		screen.spliceCells(buffer, count, command == '@')
	case 'b':
		if !buffer.canRepeat {
			screen.reject("REP without retained ASCII graphic")
			return true
		}
		graphic := buffer.repeatRune
		for range count {
			screen.printRune(graphic)
			if screen.err != nil {
				break
			}
		}
	case 'Z':
		column := max(0, ((buffer.column-1)/8-count+1)*8)
		screen.move(buffer, buffer.row, column)
	case 'r':
		screen.setScrollRegion(buffer, params)
	}
	return true
}

func (screen *bddNativeFrameDecoder) executeControl(control byte) {
	buffer := screen.active()
	switch control {
	case 0, 7, 0x7f:
	case '\r':
		screen.move(buffer, buffer.row, 0)
	case '\n', '\v', '\f':
		screen.lineFeed(buffer)
	case '\b':
		screen.move(buffer, buffer.row, max(0, buffer.column-1))
	case '\t':
		screen.move(buffer, buffer.row, (buffer.column/8+1)*8)
	default:
		screen.reject(fmt.Sprintf("control %#x", control))
	}
}

func bddNativeFrameParameter(params ansi.Params, index, fallback int) int {
	value, _, _ := params.Param(index, fallback)
	return value
}

func (screen *bddNativeFrameDecoder) executeCSI(command ansi.Cmd, params ansi.Params) {
	if screen.nonvisualCSI(command, params) {
		return
	}
	if command.Prefix() == '?' && command.Intermediate() == 0 && (command.Final() == 'h' || command.Final() == 'l') {
		screen.executeMode(params, command.Final() == 'h')
		return
	}
	if command.Prefix() != 0 || command.Intermediate() != 0 {
		screen.reject(fmt.Sprintf("CSI prefix/intermediate %q/%q/%q", command.Prefix(), command.Intermediate(), command.Final()))
		return
	}
	for _, param := range params {
		if param.HasMore() {
			screen.reject("subparameters in display operation")
			return
		}
	}
	if screen.rendererOperation(command.Final(), params) {
		return
	}
	buffer := screen.active()
	distance := max(1, bddNativeFrameParameter(params, 0, 1))
	switch command.Final() {
	case 'H', 'f':
		screen.move(buffer, distance-1, max(1, bddNativeFrameParameter(params, 1, 1))-1)
	case 'A':
		screen.move(buffer, buffer.row-distance, buffer.column)
	case 'B':
		screen.move(buffer, buffer.row+distance, buffer.column)
	case 'C':
		screen.move(buffer, buffer.row, buffer.column+distance)
	case 'D':
		screen.move(buffer, buffer.row, buffer.column-distance)
	case 'E':
		screen.move(buffer, buffer.row+distance, 0)
	case 'F':
		screen.move(buffer, buffer.row-distance, 0)
	case 'G', '`':
		screen.move(buffer, buffer.row, distance-1)
	case 'd':
		screen.move(buffer, distance-1, buffer.column)
	case 'J', 'K':
		screen.erase(command.Final(), bddNativeFrameParameter(params, 0, 0))
	case 'X':
		screen.clearRange(buffer, buffer.row, buffer.column, min(screen.width, buffer.column+distance))
	case 's':
		buffer.savedRow, buffer.savedColumn = buffer.row, buffer.column
	case 'u':
		screen.move(buffer, buffer.savedRow, buffer.savedColumn)
	default:
		screen.reject(fmt.Sprintf("CSI display operation %q", command.Final()))
	}
}

func (screen *bddNativeFrameDecoder) nonvisualCSI(command ansi.Cmd, params ansi.Params) bool {
	prefix, intermediate, final := command.Prefix(), command.Intermediate(), command.Final()
	if bddNativeNonvisualStyleCSI(command, params) {
		return true
	}
	if intermediate == 0 && final == 'c' && (prefix == 0 || prefix == '>' || prefix == '=') {
		return true // Device-attribute queries.
	}
	if intermediate == 0 && prefix == 0 && final == 'n' {
		value := bddNativeFrameParameter(params, 0, 0)
		return value == 5 || value == 6
	}
	if intermediate == 0 && prefix == '>' && final == 'm' {
		return bddNativeFrameParameter(params, 0, 0) == 4 // modifyOtherKeys input protocol.
	}
	if intermediate == 0 && final == 'u' && (prefix == '?' || prefix == '>' || prefix == '<' || prefix == '=') {
		return true // Kitty keyboard query/stack; it cannot write display cells.
	}
	if intermediate == 0 && prefix == '>' && final == 'q' {
		return true // Terminal name/version query.
	}
	return bddNativeNonvisualModeQuery(command, params)
}

func bddNativeNonvisualModeQuery(command ansi.Cmd, params ansi.Params) bool {
	prefix, intermediate, final := command.Prefix(), command.Intermediate(), command.Final()
	if intermediate == '$' && prefix == '?' && final == 'p' {
		value := bddNativeFrameParameter(params, 0, 0)
		return value == 2026 || value == 2027
	}
	return false
}

func bddNativeNonvisualStyleCSI(command ansi.Cmd, params ansi.Params) bool {
	prefix, intermediate, final := command.Prefix(), command.Intermediate(), command.Final()
	if intermediate == 0 && prefix == '?' && final == 'W' && len(params) == 1 && !params[0].HasMore() && params[0].Param(0) == 5 {
		return true // DECST8C restores the eight-column tab grid already used by this decoder.
	}
	if intermediate == 0 && prefix == 0 && final == 'm' {
		return true // SGR changes style, not the plain cell contents.
	}
	if intermediate == ' ' && prefix == 0 && final == 'q' {
		return true // Cursor shape.
	}
	return false
}

// Validate the entire list before any supported leading mode can mutate state.
func (screen *bddNativeFrameDecoder) privateModesSupported(params ansi.Params, enabled bool) bool {
	if len(params) == 0 {
		screen.reject("empty private mode")
		return false
	}
	for _, parameter := range params {
		if parameter.HasMore() {
			screen.reject("subparameters in private mode")
			return false
		}
		switch parameter.Param(0) {
		case 7, 25, 1049, 1000, 1002, 1003, 1004, 1006, 2004, 2027:
		case 2026:
			if enabled {
				screen.reject("synchronized display commit is not supported")
				return false
			}
		default:
			screen.reject(fmt.Sprintf("private mode %d", parameter.Param(0)))
			return false
		}
	}
	return true
}

func (screen *bddNativeFrameDecoder) executeMode(params ansi.Params, enabled bool) {
	if !screen.privateModesSupported(params, enabled) {
		return
	}
	for _, parameter := range params {
		switch parameter.Param(0) {
		case 7:
			buffer := screen.active()
			buffer.autoWrap = enabled
			buffer.wrapPending = false
			buffer.intendedRow, buffer.intendedColumn = buffer.row, buffer.column
		case 1049:
			if enabled && !screen.inAlternate {
				screen.alternate = newBDDNativeFrameBuffer(screen.width, screen.height)
				screen.inAlternate = true
			} else if !enabled {
				screen.inAlternate = false
			}
		case 2026:
			if enabled {
				screen.reject("synchronized display commit is not supported")
				return
			}
		case 25, 1000, 1002, 1003, 1004, 1006, 2004, 2027:
			// Cursor visibility, input reporting and width negotiation do not write cells.
		default:
			screen.reject(fmt.Sprintf("private mode %d", parameter.Param(0)))
			return
		}
	}
}

func (screen *bddNativeFrameDecoder) clearRange(buffer *bddNativeFrameBuffer, row, start, end int) {
	for column := start; column < end; column++ {
		screen.clearCell(buffer, row, column)
	}
	buffer.hasLast = false
}

func (screen *bddNativeFrameDecoder) erase(command byte, mode int) {
	if mode < 0 || mode > 2 {
		screen.reject("unsupported erasure mode")
		return
	}
	buffer := screen.active()
	if command == 'J' && mode == 2 {
		screen.fullClearSequence++
		buffer.fullClearSequence = screen.fullClearSequence
		buffer.currentOverflow = nil
	}
	first, last := buffer.row, buffer.row
	if command == 'J' {
		if mode == 0 || mode == 2 {
			last = screen.height - 1
		}
		if mode == 1 || mode == 2 {
			first = 0
		}
	}
	for row := first; row <= last; row++ {
		start, end := 0, screen.width
		if row == buffer.row && mode == 0 {
			start = buffer.column
		}
		if row == buffer.row && mode == 1 {
			end = buffer.column + 1
		}
		screen.clearRange(buffer, row, start, end)
	}
}

func (screen *bddNativeFrameDecoder) executeESC(command ansi.Cmd) {
	if command.Prefix() != 0 || command.Intermediate() != 0 {
		screen.reject("ESC intermediate or charset operation")
		return
	}
	buffer := screen.active()
	switch command.Final() {
	case '7':
		buffer.savedRow, buffer.savedColumn = buffer.row, buffer.column
	case '8':
		screen.move(buffer, buffer.savedRow, buffer.savedColumn)
	case 'D':
		screen.lineFeed(buffer)
	case 'E':
		screen.move(buffer, buffer.row, 0)
		screen.lineFeed(buffer)
	case 'M':
		if buffer.row == buffer.scrollTop {
			screen.shiftLines(buffer, buffer.scrollTop, buffer.scrollBottom, 1, true)
			screen.move(buffer, buffer.row, buffer.column)
		} else {
			screen.move(buffer, max(0, buffer.row-1), buffer.column)
		}
	case '\\': // String terminator; the parser already dispatched its payload.
	default:
		screen.reject(fmt.Sprintf("ESC operation %q", command.Final()))
	}
}

func (screen *bddNativeFrameDecoder) executeOSC(command int, _ []byte) {
	switch command {
	case 0, 1, 2, 8, 10, 11, 12, 110, 111, 112:
		// Titles, hyperlinks and colors do not produce plain display cells.
	default:
		screen.reject(fmt.Sprintf("OSC operation %d", command))
	}
}
