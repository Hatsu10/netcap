/*
* NETCAP - Traffic Analysis Framework
* Copyright (c) 2017-2020 Philipp Mieden <dreadl0ck [at] protonmail [dot] ch>
*
* THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
* WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
* MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
* ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
* WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
* ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
* OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
 */

package decoder

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"errors"
	"github.com/dreadl0ck/netcap/reassembly"
	"github.com/dreadl0ck/netcap/utils"
	"io"
	"net/http"
	"net/textproto"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/dreadl0ck/cryptoutils"
	"github.com/mgutz/ansi"

	"compress/gzip"

	"github.com/dreadl0ck/netcap/types"
)

/*
* POP3 part
 */

var pop3Debug = false

type pop3Reader struct {
	parent *tcpConnection

	pop3Requests  []*types.POP3Request
	pop3Responses []*types.POP3Response
	reqIndex      int
	resIndex      int

	user, pass, token string
}

func (h *pop3Reader) Decode(c2s, s2c Stream) {

	var (
		buf         bytes.Buffer
		previousDir reassembly.TCPFlowDirection
	)
	if len(h.parent.merged) > 0 {
		previousDir = h.parent.merged[0].dir
	}

	// parse conversation
	for _, d := range h.parent.merged {

		if d.dir == previousDir {
			//fmt.Println(d.dir, "collect", len(d.raw), d.ac.GetCaptureInfo().Timestamp)
			buf.Write(d.raw)
		} else {
			var err error

			//fmt.Println(hex.Dump(buf.Bytes()))

			b := bufio.NewReader(&buf)
			if previousDir == reassembly.TCPDirClientToServer {
				for err != io.EOF && err != io.ErrUnexpectedEOF {
					err = h.readRequest(b, c2s)
				}
			} else {
				for err != io.EOF && err != io.ErrUnexpectedEOF {
					err = h.readResponse(b, s2c)
				}
			}
			//if err != nil {
			//	fmt.Println(err)
			//}
			buf.Reset()
			previousDir = d.dir

			buf.Write(d.raw)
			continue
		}
	}
	var err error
	b := bufio.NewReader(&buf)
	if previousDir == reassembly.TCPDirClientToServer {
		for err != io.EOF && err != io.ErrUnexpectedEOF {
			err = h.readRequest(b, c2s)
		}
	} else {
		for err != io.EOF && err != io.ErrUnexpectedEOF {
			err = h.readResponse(b, s2c)
		}
	}
	//if err != nil {
	//	fmt.Println(err)
	//}

	//fmt.Println("POP3", h.parent.ident, len(h.pop3Responses), len(h.pop3Requests))

	mails, user, pass, token := h.parseMails()
	pop3Msg := &types.POP3{
		Timestamp: h.parent.firstPacket.String(),
		ClientIP:  h.parent.net.Src().String(),
		ServerIP:  h.parent.net.Dst().String(),
		AuthToken: token,
		User:      user,
		Pass:      pass,
		Mails:     mails,
	}

	if user != "" || pass != "" {
		writeCredentials(&types.Credentials{
			Timestamp: h.parent.firstPacket.String(),
			Service:   "POP3",
			Flow:      h.parent.ident,
			User:      user,
			Password:  pass,
		})
	}

	// export metrics if configured
	if pop3Decoder.export {
		pop3Msg.Inc()
	}

	// write record to disk
	atomic.AddInt64(&pop3Decoder.numRecords, 1)
	err = pop3Decoder.writer.Write(pop3Msg)
	if err != nil {
		errorMap.Inc(err.Error())
	}

	// inserts a newline to increase readability
	mailDebug()
}

// TODO: use saveFile to extract attachments
func (h *pop3Reader) saveFile(source, name string, err error, body []byte, encoding []string) error {

	// prevent saving zero bytes
	if len(body) == 0 {
		return nil
	}

	if name == "" || name == "/" {
		name = "unknown"
	}

	var (
		fileName string

		// detected content type
		ctype = http.DetectContentType(body)

		// root path
		root = path.Join(c.FileStorage, ctype)

		// file extension
		ext = fileExtensionForContentType(ctype)

		// file basename
		base = filepath.Clean(name+"-"+path.Base(h.parent.ident)) + ext
	)
	if err != nil {
		base = "incomplete-" + base
	}
	if filepath.Ext(name) == "" {
		fileName = name + ext
	} else {
		fileName = name
	}

	// make sure root path exists
	os.MkdirAll(root, defaultDirectoryPermission)
	base = path.Join(root, base)
	if len(base) > 250 {
		base = base[:250] + "..."
	}
	if base == c.FileStorage {
		base = path.Join(c.FileStorage, "noname")
	}
	var (
		target = base
		n      = 0
	)
	for {
		_, errStat := os.Stat(target)
		if errStat != nil {
			break
		}

		if err != nil {
			target = path.Join(root, filepath.Clean("incomplete-"+name+"-"+h.parent.ident)+"-"+strconv.Itoa(n)+fileExtensionForContentType(ctype))
		} else {
			target = path.Join(root, filepath.Clean(name+"-"+h.parent.ident)+"-"+strconv.Itoa(n)+fileExtensionForContentType(ctype))
		}

		n++
	}

	//fmt.Println("saving file:", target)

	f, err := os.Create(target)
	if err != nil {
		logReassemblyError("POP3-create", "Cannot create %s: %s\n", target, err)
		return err
	}

	// explicitly declare io.Reader interface
	var r io.Reader

	// now assign a new buffer
	r = bytes.NewBuffer(body)
	if len(encoding) > 0 && (encoding[0] == "gzip" || encoding[0] == "deflate") {
		r, err = gzip.NewReader(r)
		if err != nil {
			logReassemblyError("POP3-gunzip", "Failed to gzip decode: %s", err)
		}
	}
	if err == nil {
		w, err := io.Copy(f, r)
		if _, ok := r.(*gzip.Reader); ok {
			r.(*gzip.Reader).Close()
		}
		f.Close()
		if err != nil {
			logReassemblyError("POP3-save", "%s: failed to save %s (l:%d): %s\n", h.parent.ident, target, w, err)
		} else {
			logReassemblyInfo("%s: Saved %s (l:%d)\n", h.parent.ident, target, w)
		}
	}

	// write file to disk
	writeFile(&types.File{
		Timestamp:   h.parent.firstPacket.String(),
		Name:        fileName,
		Length:      int64(len(body)),
		Hash:        hex.EncodeToString(cryptoutils.MD5Data(body)),
		Location:    target,
		Ident:       h.parent.ident,
		Source:      source,
		ContentType: ctype,
		Context: &types.PacketContext{
			SrcIP:   h.parent.net.Src().String(),
			DstIP:   h.parent.net.Dst().String(),
			SrcPort: h.parent.transport.Src().String(),
			DstPort: h.parent.transport.Dst().String(),
		},
	})

	return nil
}

func mailDebug(args ...interface{}) {
	if pop3Debug {
		utils.DebugLog.Println(args...)
	}
}

func (h *pop3Reader) readRequest(b *bufio.Reader, c2s Stream) error {

	tp := textproto.NewReader(b)

	// Parse the first line of the response.
	line, err := tp.ReadLine()
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return err
	} else if err != nil {
		utils.DebugLog.Printf("POP3/%s Request error: %s (%v,%+v)\n", h.parent.ident, err, err, err)
		return err
	}

	mailDebug(ansi.Red, h.parent.ident, "readRequest", line, ansi.Reset)

	cmd, args := getCommand(line)

	h.parent.Lock()
	h.pop3Requests = append(h.pop3Requests, &types.POP3Request{
		Command:  cmd,
		Argument: strings.Join(args, " "),
	})
	h.parent.Unlock()

	if cmd == "QUIT" {
		return io.EOF
	}

	return nil
}

func (h *pop3Reader) readResponse(b *bufio.Reader, s2c Stream) error {

	tp := textproto.NewReader(b)

	// Parse the first line of the response.
	line, err := tp.ReadLine()
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return err
	} else if err != nil {
		logReassemblyError("POP3-response", "POP3/%s Response error: %s (%v,%+v)\n", h.parent.ident, err, err, err)
		return err
	}

	mailDebug(ansi.Blue, h.parent.ident, "readResponse", line, ansi.Reset)

	cmd, args := getCommand(line)

	if validPop3ServerCommand(cmd) {
		h.parent.Lock()
		h.pop3Responses = append(h.pop3Responses, &types.POP3Response{
			Command: cmd,
			Message: strings.Join(args, " "),
		})
		h.parent.Unlock()
	} else {
		if line == "" {
			line = "\n"
		}
		h.parent.Lock()
		h.pop3Responses = append(h.pop3Responses, &types.POP3Response{
			Message: line,
		})
		h.parent.Unlock()
	}

	if line == "-ERR authentication failed" || strings.Contains(line, "signing off") {
		return io.EOF
	}

	return nil
}

// cuts the line into command and arguments
func getCommand(line string) (string, []string) {
	line = strings.Trim(line, "\r \n")
	cmd := strings.Split(line, " ")
	return cmd[0], cmd[1:]
}

func validPop3ServerCommand(cmd string) bool {
	switch cmd {
	case ".":
		fallthrough
	case "+":
		fallthrough
	case "+OK":
		fallthrough
	case "-ERR":
		fallthrough
	case "TOP":
		fallthrough
	case "USER":
		fallthrough
	case "UIDL":
		fallthrough
	case "STLS":
		fallthrough
	case "SASL":
		fallthrough
	case "IMPLEMENTATION":
		return true
	default:
		return false
	}
}

type POP3State int

const (
	StateNotAuthenticated POP3State = iota
	StateNotIdentified
	StateAuthenticated
	StateDataTransfer
)

func (h *pop3Reader) parseMails() (mails []*types.Mail, user, pass, token string) {

	if len(h.pop3Responses) == 0 || len(h.pop3Requests) == 0 {
		return
	}

	// check if server hello
	serverHello := h.pop3Responses[0]
	if serverHello.Command != "+OK" {
		return
	}
	if !strings.HasPrefix(serverHello.Message, "POP server ready") {
		return
	}

	var (
		state    POP3State = StateNotAuthenticated
		numMails int
		next     = func() *types.POP3Request {
			return h.pop3Requests[h.reqIndex]
		}
		mailBuf string
		r       *types.POP3Request
	)

	for {
		if h.reqIndex == len(h.pop3Requests) {
			return
		}
		r = next()
		h.reqIndex++
		//fmt.Println("CMD", r.Command, r.Argument, "h.resIndex", h.resIndex)

		switch state {
		case StateAuthenticated:
			switch r.Command {
			case "STAT":
				h.resIndex++
				continue
			case "LIST", "UIDL":
				var n int
				// ensure safe array access
				if len(h.pop3Responses) < h.resIndex {
					time.Sleep(100 * time.Millisecond) // not there yet? wait a little and retry
					if len(h.pop3Responses) < h.resIndex {
						continue
					}
				}
				for _, reply := range h.pop3Responses[h.resIndex:] {
					if reply.Command == "." {
						numMails++
						h.resIndex++
						break
					}
					n++
				}
				h.resIndex = h.resIndex + n
				continue
			case "RETR":
				var n int
				// ensure safe array access
				if len(h.pop3Responses) < h.resIndex {
					time.Sleep(100 * time.Millisecond) // not there yet? wait a little and retry
					if len(h.pop3Responses) < h.resIndex {
						continue
					}
				}
				for _, reply := range h.pop3Responses[h.resIndex:] {
					if reply.Command == "." {
						mails = append(mails, h.parseMail([]byte(mailBuf)))
						mailBuf = ""
						numMails++
						h.resIndex++
						break
					}
					mailBuf += reply.Message + "\n"
					n++
				}
				h.resIndex = h.resIndex + n
				continue
			case "QUIT":
				return
			}
		case StateNotAuthenticated:
			switch r.Command {
			case "USER":
				if len(h.pop3Responses) <= h.resIndex+1 {
					continue
				}
				reply := h.pop3Responses[h.resIndex+1]
				if reply.Command == "+OK" {
					user = r.Argument
				}
				h.resIndex++
				continue
			case "CAPA":
				var n int
				for _, reply := range h.pop3Responses[h.resIndex:] {
					if reply.Command == "." {
						numMails++
						h.resIndex++
						break
					}
					n++
				}
				h.resIndex = h.resIndex + n
				continue
			case "AUTH":
				if len(h.pop3Responses) <= h.resIndex+1 {
					continue
				}
				reply := h.pop3Responses[h.resIndex+1]
				if reply.Command == "+OK" {
					state = StateAuthenticated
					if len(h.pop3Requests) < h.reqIndex {
						r = h.pop3Requests[h.reqIndex]
						if r != nil {
							token = r.Command
						}
					}
				}
				h.resIndex++
				continue
			case "PASS":
				if len(h.pop3Responses) <= h.resIndex+1 {
					continue
				}
				reply := h.pop3Responses[h.resIndex+1]
				if reply.Command == "+OK" {
					state = StateAuthenticated
					pass = r.Argument
				}
				h.resIndex++
				continue
			case "APOP": // example: APOP mrose c4c9334bac560ecc979e58001b3e22fb
				if len(h.pop3Responses) <= h.resIndex+1 {
					continue
				}
				reply := h.pop3Responses[h.resIndex+1]
				if reply.Command == "+OK" {
					state = StateAuthenticated
					parts := strings.Split(r.Argument, " ")
					if len(parts) > 1 {
						user = parts[0]
						token = parts[1]
					}
				}
				h.resIndex++
				continue
			case "QUIT":
				return
			}
		}
		h.resIndex++
	}
}

func splitMailHeaderAndBody(buf []byte) (map[string]string, string) {

	var (
		header      = make(map[string]string)
		r           = textproto.NewReader(bufio.NewReader(bytes.NewReader(buf)))
		body        string
		lastHeader  string
		collectBody bool
	)

	for {
		line, err := r.ReadLine()
		if err != nil {
			return header, body
		}

		if collectBody {
			body += line + "\n"
			continue
		}

		if line == "" {
			continue
		}

		parts := strings.Split(line, ": ")
		if len(parts) == 0 {
			header[lastHeader] += "\n" + line
			continue
		}

		// should be an uppercase char if header field
		// multi line values start with a whitespace
		if len(parts[0]) > 0 && unicode.IsUpper(rune(parts[0][0])) {
			if parts[0] == "Envelope-To" {
				collectBody = true
			}
			header[parts[0]] = strings.Join(parts[1:], ": ")
			lastHeader = parts[0]
		} else {
			// multiline
			header[lastHeader] += "\n" + line
		}
	}
}

func (h *pop3Reader) parseMail(buf []byte) *types.Mail {

	mailDebug(ansi.Yellow, "parseMail", h.parent.ident, ansi.Reset)

	header, body := splitMailHeaderAndBody(buf)
	mail := &types.Mail{
		ReturnPath:      header["Return-Path"],
		DeliveryDate:    header["Delivery-Date"],
		From:            header["From"],
		To:              header["To"],
		CC:              header["CC"],
		Subject:         header["Subject"],
		Date:            header["Date"],
		MessageID:       header["Message-ID"],
		References:      header["References"],
		InReplyTo:       header["In-Reply-To"],
		ContentLanguage: header["Content-Language"],
		XOriginatingIP:  header["x-originating-ip"],
		ContentType:     header["Content-Type"],
		EnvelopeTo:      header["Envelope-To"],
		Body:            h.parseParts(body),
	}
	for _, p := range mail.Body {
		if strings.Contains(p.Header["Content-Disposition"], "attachment") {
			mail.HasAttachments = true
			break
		}
	}
	return mail
}

const partIdent = "------=_Part_"

func (h *pop3Reader) parseParts(body string) []*types.MailPart {

	var (
		parts        []*types.MailPart
		currentPart  *types.MailPart
		parsePayload bool
		tr           = textproto.NewReader(bufio.NewReader(bytes.NewReader([]byte(body))))
	)

	mailDebug(ansi.White, h.parent.ident, body)

	for {
		line, err := tr.ReadLine()
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				break
			} else {
				mailDebug("failed to read line: ", err)
				return parts
			}
		}

		mailDebug(ansi.Green, h.parent.ident, "readLine", line)

		if currentPart != nil {
			if parsePayload {
				// check if its an end marker for the current part
				if strings.HasSuffix(line, currentPart.ID+"--") {
					mailDebug(ansi.Cyan, "end", currentPart.ID, ansi.Reset)
					parts = append(parts, copyMailPart(currentPart))
					parsePayload = false
					currentPart = nil

					// check if its the start of another part, marker type 1
				} else if strings.HasPrefix(line, partIdent) {
					parts = append(parts, copyMailPart(currentPart))
					currentPart = &types.MailPart{
						ID:     strings.TrimPrefix(line, partIdent),
						Header: make(map[string]string),
					}
					parsePayload = false
					mailDebug(ansi.Red, "start", currentPart.ID, ansi.Reset)

					// second type of start marker
				} else if strings.HasPrefix(line, "--") && len(line) > 25 && !strings.Contains(line, ">") {
					parts = append(parts, copyMailPart(currentPart))
					currentPart = &types.MailPart{
						ID:     strings.TrimPrefix(line, "--"),
						Header: make(map[string]string),
					}
					parsePayload = false
					mailDebug(ansi.Red, "start", currentPart.ID, ansi.Reset)

					// its content
				} else {
					currentPart.Content += line + "\n"
					mailDebug(ansi.Blue, "adding content", line, ansi.Reset)
				}
				continue
			}
			pts := strings.Split(line, ": ")
			if len(pts) == 2 {
				currentPart.Header[pts[0]] = pts[1]
				mailDebug(ansi.Yellow, "parsed header field", pts[0], ansi.Reset)
			} else {
				pts = strings.Split(line, "filename=")
				if len(pts) == 2 {
					currentPart.Filename = strings.Trim(pts[1], "\"")
					mailDebug(ansi.Yellow, "parsed filename field", currentPart.Filename, ansi.Reset)
				}
			}
			if line == "\n" || line == "" {
				parsePayload = true
				mailDebug(ansi.Green, "start parsing payload", ansi.Reset)
			}
			continue
		}
		// start marker type 1
		if strings.HasPrefix(line, partIdent) {
			currentPart = &types.MailPart{
				ID:     strings.TrimPrefix(line, partIdent),
				Header: make(map[string]string),
			}
			mailDebug(ansi.Red, "start", currentPart.ID, ansi.Reset)
			continue
		}
		// start marker type 2
		if strings.HasPrefix(line, "--") && len(line) > 31 && !strings.Contains(line, ">") {
			currentPart = &types.MailPart{
				ID:     strings.TrimPrefix(line, "--"),
				Header: make(map[string]string),
			}
			mailDebug(ansi.Red, "start", currentPart.ID, ansi.Reset)
			continue
		}

		// single parts have no markers
		mailDebug(ansi.Red, "no marker found", line)
		currentPart = &types.MailPart{
			ID:     "none",
			Header: make(map[string]string),
		}
		pts := strings.Split(line, ": ")
		if len(pts) == 2 {
			currentPart.Header[pts[0]] = pts[1]
			mailDebug(ansi.Yellow, "parsed header field", pts[0], ansi.Reset)
		} else {
			pts = strings.Split(line, "filename=")
			if len(pts) == 2 {
				currentPart.Filename = strings.Trim(pts[1], "\"")
				mailDebug(ansi.Yellow, "parsed filename field", currentPart.Filename, ansi.Reset)
			}
		}
		if line == "\n" || line == "" {
			parsePayload = true
			mailDebug(ansi.Green, "start parsing payload", ansi.Reset)
		}
	}

	return parts
}

func copyMailPart(part *types.MailPart) *types.MailPart {
	return &types.MailPart{
		ID:       part.ID,
		Header:   part.Header,
		Content:  part.Content,
		Filename: part.Filename,
	}
}

// TODO: write unit test for this
//< +OK POP3 server ready <mailserver.mydomain.com>
//>USER user1
//< +OK
//>PASS <password>
//<+OK user1's maildrop has 2 messages (320 octets)
//> STAT
//< +OK 2 320
//> LIST
//< +OK 2 messages

// save token
// request: AUTH PLAIN
// next command is token

// parse user and MD5 from APOP cmd
//S: +OK POP3 server ready <1896.697170952@dbc.mtview.ca.us>
//C: APOP mrose c4c9334bac560ecc979e58001b3e22fb
//S: +OK maildrop has 1 message (369 octets)

// save USER name and PASS
//Possible Responses:
//+OK name is a valid mailbox
//-ERR never heard of mailbox name
//
//Examples:
//C: USER frated
//S: -ERR sorry, no mailbox for frated here
//...
//C: USER mrose
//S: +OK mrose is a real hoopy frood

// test wrong and corrrect PASS cmd usage
//Possible Responses:
//+OK maildrop locked and ready
//-ERR invalid password
//-ERR unable to lock maildrop
//
//Examples:
//C: USER mrose
//S: +OK mrose is a real hoopy frood
//C: PASS secret
//S: -ERR maildrop already locked
//...
//C: USER mrose
//S: +OK mrose is a real hoopy frood
//C: PASS secret
//S: +OK mrose's maildrop has 2 messages (320 octets)