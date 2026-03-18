package ftp

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path"
	"strings"
	"time"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/file"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/vault"
)

// maxDocSize is the maximum document size (10 MB).
const maxDocSize = 10 << 20

// session handles a single FTP client connection.
type session struct {
	conn       net.Conn
	reader     *bufio.Reader
	dbClient   *db.Client
	docService *file.Service
	vaultSvc   *vault.Service
	noAuth     bool
	logger     *slog.Logger

	// auth state
	user string
	ac   *auth.AuthContext

	// navigation state
	cwd string // current working directory, e.g. "/" or "/vault/path"

	// passive mode listener
	pasvLn  net.Listener
	pasvMin int
	pasvMax int

	// rename state (RNFR/RNTO pair)
	renameFrom string

	// transfer type (A = ASCII, I = binary)
	transferType byte
}

func newSession(
	conn net.Conn,
	dbClient *db.Client,
	docService *file.Service,
	vaultSvc *vault.Service,
	noAuth bool,
	pasvMin, pasvMax int,
) *session {
	return &session{
		conn:         conn,
		reader:       bufio.NewReader(conn),
		dbClient:     dbClient,
		docService:   docService,
		vaultSvc:     vaultSvc,
		noAuth:       noAuth,
		logger:       slog.Default().With("component", "ftp", "remote", conn.RemoteAddr()),
		cwd:          "/",
		transferType: 'A',
		pasvMin:      pasvMin,
		pasvMax:      pasvMax,
	}
}

func (s *session) run() {
	defer s.conn.Close()
	defer s.closePasv()

	s.reply(220, "Know FTP server ready")

	for {
		if err := s.conn.SetDeadline(time.Now().Add(5 * time.Minute)); err != nil {
			return
		}

		line, err := s.reader.ReadString('\n')
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
				s.logger.Debug("read error", "error", err)
			}
			return
		}

		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}

		cmd, arg := parseCommand(line)
		s.logger.Debug("command", "cmd", cmd, "arg", arg)

		if !s.handleCommand(cmd, arg) {
			return // QUIT
		}
	}
}

// parseCommand splits "CMD arg" into (CMD, arg).
func parseCommand(line string) (string, string) {
	parts := strings.SplitN(line, " ", 2)
	cmd := strings.ToUpper(parts[0])
	arg := ""
	if len(parts) > 1 {
		arg = parts[1]
	}
	return cmd, arg
}

// handleCommand dispatches a single FTP command. Returns false on QUIT.
func (s *session) handleCommand(cmd, arg string) bool {
	// Commands allowed before authentication
	switch cmd {
	case "USER":
		s.handleUser(arg)
		return true
	case "PASS":
		s.handlePass(arg)
		return true
	case "QUIT":
		s.reply(221, "Goodbye")
		return false
	case "AUTH":
		s.reply(502, "TLS not supported")
		return true
	case "FEAT":
		s.handleFeat()
		return true
	case "SYST":
		s.reply(215, "UNIX Type: L8")
		return true
	case "NOOP":
		s.reply(200, "OK")
		return true
	}

	// Remaining commands require authentication
	if s.ac == nil {
		s.reply(530, "Please login first")
		return true
	}

	switch cmd {
	case "PWD", "XPWD":
		s.reply(257, fmt.Sprintf("%q is the current directory", s.cwd))
	case "CWD", "XCWD":
		s.handleCwd(arg)
	case "CDUP", "XCUP":
		s.handleCwd("..")
	case "TYPE":
		s.handleType(arg)
	case "MODE":
		if strings.ToUpper(arg) == "S" {
			s.reply(200, "Mode set to Stream")
		} else {
			s.reply(504, "Only stream mode supported")
		}
	case "STRU":
		if strings.ToUpper(arg) == "F" {
			s.reply(200, "Structure set to File")
		} else {
			s.reply(504, "Only file structure supported")
		}
	case "PASV":
		s.handlePasv()
	case "EPSV":
		s.handleEpsv()
	case "LIST":
		s.handleList(arg)
	case "MLSD":
		s.handleMlsd(arg)
	case "NLST":
		s.handleNlst(arg)
	case "RETR":
		s.handleRetr(arg)
	case "STOR":
		s.handleStor(arg)
	case "DELE":
		s.handleDele(arg)
	case "MKD", "XMKD":
		s.handleMkd(arg)
	case "RMD", "XRMD":
		s.handleRmd(arg)
	case "RNFR":
		s.handleRnfr(arg)
	case "RNTO":
		s.handleRnto(arg)
	case "SIZE":
		s.handleSize(arg)
	case "MDTM":
		s.handleMdtm(arg)
	case "OPTS":
		if strings.HasPrefix(strings.ToUpper(arg), "UTF8") {
			s.reply(200, "UTF8 mode enabled")
		} else {
			s.reply(502, "Option not supported")
		}
	case "PORT", "EPRT":
		s.reply(502, "Active mode not supported; use PASV or EPSV")
	default:
		s.reply(502, fmt.Sprintf("Command %s not implemented", cmd))
	}

	return true
}

// reply sends an FTP response line.
func (s *session) reply(code int, msg string) {
	line := fmt.Sprintf("%d %s\r\n", code, msg)
	if _, err := s.conn.Write([]byte(line)); err != nil {
		s.logger.Debug("write error", "error", err)
	}
}

// replyMulti sends a multiline FTP response.
func (s *session) replyMulti(code int, lines []string) {
	var buf bytes.Buffer
	for i, line := range lines {
		if i < len(lines)-1 {
			fmt.Fprintf(&buf, "%d-%s\r\n", code, line)
		} else {
			fmt.Fprintf(&buf, "%d %s\r\n", code, line)
		}
	}
	if _, err := s.conn.Write(buf.Bytes()); err != nil {
		s.logger.Debug("write error", "error", err)
	}
}

// handleUser handles the USER command.
func (s *session) handleUser(username string) {
	s.user = username
	s.ac = nil
	s.reply(331, "Password required")
}

// handlePass handles the PASS command.
func (s *session) handlePass(password string) {
	if s.user == "" {
		s.reply(503, "Send USER first")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ac, err := auth.Authenticate(ctx, s.dbClient, password, s.noAuth)
	if err != nil {
		s.logger.Warn("auth failed", "user", s.user, "error", err)
		s.reply(530, "Login incorrect")
		return
	}

	s.ac = &ac
	s.logger.Info("authenticated", "user", s.user)
	s.reply(230, "Login successful")
}

// handleFeat advertises supported FTP extensions.
func (s *session) handleFeat() {
	lines := []string{
		"Features:",
		" PASV",
		" EPSV",
		" SIZE",
		" MDTM",
		" MLSD",
		" UTF8",
		"End",
	}
	s.replyMulti(211, lines)
}

// handleType sets the transfer type.
func (s *session) handleType(arg string) {
	switch strings.ToUpper(arg) {
	case "A", "A N":
		s.transferType = 'A'
		s.reply(200, "Type set to ASCII")
	case "I", "L 8":
		s.transferType = 'I'
		s.reply(200, "Type set to Binary")
	default:
		s.reply(504, "Unsupported type")
	}
}

// handleCwd changes the working directory.
func (s *session) handleCwd(arg string) {
	target := s.resolvePath(arg)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Root is always valid
	if target == "/" {
		s.cwd = "/"
		s.reply(250, "Directory changed to /")
		return
	}

	vaultName, docPath := parsePath(target)

	vaultID, err := s.resolveVault(ctx, vaultName)
	if err != nil {
		s.reply(550, "No such directory")
		return
	}

	// Vault root is always valid
	if docPath == "/" {
		s.cwd = target
		s.reply(250, fmt.Sprintf("Directory changed to %s", target))
		return
	}

	// Check if folder exists
	folder, err := s.dbClient.GetFolderByPath(ctx, vaultID, docPath)
	if err != nil {
		s.reply(550, "Directory lookup failed")
		return
	}
	if folder == nil {
		s.reply(550, "No such directory")
		return
	}

	s.cwd = target
	s.reply(250, fmt.Sprintf("Directory changed to %s", target))
}

// handlePasv enters passive mode (PASV).
func (s *session) handlePasv() {
	ln, err := s.openPasvListener()
	if err != nil {
		s.reply(425, "Cannot open passive connection")
		return
	}

	addr := ln.Addr().(*net.TCPAddr)
	ip := addr.IP.To4()
	if ip == nil {
		ip = net.IPv4(127, 0, 0, 1)
	}

	p1 := addr.Port / 256
	p2 := addr.Port % 256
	s.reply(227, fmt.Sprintf("Entering Passive Mode (%d,%d,%d,%d,%d,%d)",
		ip[0], ip[1], ip[2], ip[3], p1, p2))
}

// handleEpsv enters extended passive mode (EPSV).
func (s *session) handleEpsv() {
	ln, err := s.openPasvListener()
	if err != nil {
		s.reply(425, "Cannot open passive connection")
		return
	}

	port := ln.Addr().(*net.TCPAddr).Port
	s.reply(229, fmt.Sprintf("Entering Extended Passive Mode (|||%d|)", port))
}

// openPasvListener creates a TCP listener for passive data connections.
func (s *session) openPasvListener() (net.Listener, error) {
	s.closePasv()

	// Use the host from the control connection
	host, _, _ := net.SplitHostPort(s.conn.LocalAddr().String())

	// Try ports in the configured range
	if s.pasvMin > 0 && s.pasvMax > 0 {
		for port := s.pasvMin; port <= s.pasvMax; port++ {
			ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
			if err == nil {
				s.pasvLn = ln
				return ln, nil
			}
		}
		return nil, fmt.Errorf("no available passive ports in range %d-%d", s.pasvMin, s.pasvMax)
	}

	// No range configured — let OS pick
	ln, err := net.Listen("tcp", host+":0")
	if err != nil {
		return nil, fmt.Errorf("open passive listener: %w", err)
	}
	s.pasvLn = ln
	return ln, nil
}

// acceptDataConn accepts a single data connection from the passive listener.
func (s *session) acceptDataConn() (net.Conn, error) {
	if s.pasvLn == nil {
		return nil, fmt.Errorf("no passive connection; send PASV first")
	}

	if err := s.pasvLn.(*net.TCPListener).SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
		return nil, fmt.Errorf("set deadline: %w", err)
	}

	conn, err := s.pasvLn.Accept()
	if err != nil {
		return nil, fmt.Errorf("accept data connection: %w", err)
	}

	// Close the passive listener after accepting one connection
	s.closePasv()

	return conn, nil
}

// closePasv closes the current passive listener if one exists.
func (s *session) closePasv() {
	if s.pasvLn != nil {
		s.pasvLn.Close()
		s.pasvLn = nil
	}
}

// handleList sends a directory listing in Unix ls -l format.
func (s *session) handleList(arg string) {
	// Strip common flags like -la, -a that clients send
	dirPath := arg
	for strings.HasPrefix(dirPath, "-") {
		parts := strings.SplitN(dirPath, " ", 2)
		if len(parts) > 1 {
			dirPath = parts[1]
		} else {
			dirPath = ""
		}
	}

	target := s.cwd
	if dirPath != "" {
		target = s.resolvePath(dirPath)
	}

	entries, err := s.listDir(target)
	if err != nil {
		s.reply(550, "Directory listing failed")
		return
	}

	s.reply(150, "Opening data connection for directory listing")
	dataConn, err := s.acceptDataConn()
	if err != nil {
		s.reply(425, "Cannot open data connection")
		return
	}
	defer dataConn.Close()

	for _, e := range entries {
		perm := "-rw-r--r--"
		if e.isDir {
			perm = "drwxr-xr-x"
		}
		line := fmt.Sprintf("%s 1 owner group %12d %s %s\r\n",
			perm,
			e.size,
			e.modTime.Format("Jan 02 15:04"),
			e.name,
		)
		if _, writeErr := dataConn.Write([]byte(line)); writeErr != nil {
			s.logger.Debug("data write error", "error", writeErr)
			break
		}
	}

	s.reply(226, "Transfer complete")
}

// handleMlsd sends directory listing in machine-readable format.
func (s *session) handleMlsd(arg string) {
	target := s.cwd
	if arg != "" {
		target = s.resolvePath(arg)
	}

	entries, err := s.listDir(target)
	if err != nil {
		s.reply(550, "Directory listing failed")
		return
	}

	s.reply(150, "Opening data connection for MLSD")
	dataConn, err := s.acceptDataConn()
	if err != nil {
		s.reply(425, "Cannot open data connection")
		return
	}
	defer dataConn.Close()

	for _, e := range entries {
		entryType := "file"
		if e.isDir {
			entryType = "dir"
		}
		line := fmt.Sprintf("type=%s;size=%d;modify=%s; %s\r\n",
			entryType,
			e.size,
			e.modTime.UTC().Format("20060102150405"),
			e.name,
		)
		if _, writeErr := dataConn.Write([]byte(line)); writeErr != nil {
			s.logger.Debug("data write error", "error", writeErr)
			break
		}
	}

	s.reply(226, "Transfer complete")
}

// handleNlst sends a bare filename listing.
func (s *session) handleNlst(arg string) {
	target := s.cwd
	if arg != "" {
		target = s.resolvePath(arg)
	}

	entries, err := s.listDir(target)
	if err != nil {
		s.reply(550, "Directory listing failed")
		return
	}

	s.reply(150, "Opening data connection for NLST")
	dataConn, err := s.acceptDataConn()
	if err != nil {
		s.reply(425, "Cannot open data connection")
		return
	}
	defer dataConn.Close()

	for _, e := range entries {
		if _, writeErr := fmt.Fprintf(dataConn, "%s\r\n", e.name); writeErr != nil {
			break
		}
	}

	s.reply(226, "Transfer complete")
}

// handleRetr sends a file to the client.
func (s *session) handleRetr(arg string) {
	filePath := s.resolvePath(arg)
	vaultName, docPath := parsePath(filePath)
	if vaultName == "" || docPath == "/" {
		s.reply(550, "Not a file")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	vaultID, err := s.resolveVault(ctx, vaultName)
	if err != nil {
		s.reply(550, "Vault not found")
		return
	}

	doc, err := s.dbClient.GetFileByPath(ctx, vaultID, docPath)
	if err != nil {
		s.reply(550, "Read failed")
		return
	}
	if doc == nil {
		s.reply(550, "File not found")
		return
	}

	s.reply(150, fmt.Sprintf("Opening data connection for %s (%d bytes)", arg, len(doc.Content)))
	dataConn, err := s.acceptDataConn()
	if err != nil {
		s.reply(425, "Cannot open data connection")
		return
	}
	defer dataConn.Close()

	if _, writeErr := dataConn.Write([]byte(doc.Content)); writeErr != nil {
		s.logger.Debug("data write error", "error", writeErr)
		s.reply(426, "Transfer aborted")
		return
	}

	s.reply(226, "Transfer complete")
}

// handleStor receives a file from the client.
func (s *session) handleStor(arg string) {
	filePath := s.resolvePath(arg)
	vaultName, docPath := parsePath(filePath)
	if vaultName == "" {
		s.reply(553, "Invalid path")
		return
	}

	if !isMarkdownFile(docPath) {
		s.reply(553, "Only markdown files (.md) are allowed")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	vaultID, err := s.resolveVault(ctx, vaultName)
	if err != nil {
		s.reply(550, "Vault not found")
		return
	}

	s.reply(150, "Ready to receive data")
	dataConn, err := s.acceptDataConn()
	if err != nil {
		s.reply(425, "Cannot open data connection")
		return
	}
	defer dataConn.Close()

	data, err := io.ReadAll(io.LimitReader(dataConn, maxDocSize+1))
	if err != nil {
		s.reply(426, "Transfer aborted")
		return
	}
	if len(data) > maxDocSize {
		s.reply(552, fmt.Sprintf("File too large (max %d bytes)", maxDocSize))
		return
	}

	_, err = s.docService.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    docPath,
		Content: string(data),
	})
	if err != nil {
		s.logger.Error("save failed", "path", docPath, "vault", vaultID, "error", err)
		s.reply(550, "Save failed")
		return
	}

	s.logger.Info("document saved", "path", docPath, "vault", vaultID, "size", len(data))
	s.reply(226, "Transfer complete")
}

// handleDele deletes a file.
func (s *session) handleDele(arg string) {
	filePath := s.resolvePath(arg)
	vaultName, docPath := parsePath(filePath)
	if vaultName == "" || docPath == "/" {
		s.reply(550, "Invalid path")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	vaultID, err := s.resolveVault(ctx, vaultName)
	if err != nil {
		s.reply(550, "Vault not found")
		return
	}

	if err := s.docService.Delete(ctx, vaultID, docPath); err != nil {
		s.reply(550, "Delete failed")
		return
	}

	s.reply(250, "File deleted")
}

// handleMkd creates a directory.
func (s *session) handleMkd(arg string) {
	dirPath := s.resolvePath(arg)
	vaultName, docPath := parsePath(dirPath)
	if vaultName == "" {
		s.reply(550, "Cannot create vault via FTP")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	vaultID, err := s.resolveVault(ctx, vaultName)
	if err != nil {
		s.reply(550, "Vault not found")
		return
	}

	if docPath == "/" {
		s.reply(257, fmt.Sprintf("%q directory already exists", dirPath))
		return
	}

	if _, err := s.vaultSvc.CreateFolder(ctx, vaultID, docPath); err != nil {
		s.reply(550, "Create directory failed")
		return
	}

	s.reply(257, fmt.Sprintf("%q directory created", dirPath))
}

// handleRmd removes a directory.
func (s *session) handleRmd(arg string) {
	dirPath := s.resolvePath(arg)
	vaultName, docPath := parsePath(dirPath)
	if vaultName == "" || docPath == "/" {
		s.reply(550, "Cannot remove vault root")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	vaultID, err := s.resolveVault(ctx, vaultName)
	if err != nil {
		s.reply(550, "Vault not found")
		return
	}

	if err := s.vaultSvc.DeleteFolder(ctx, vaultID, docPath); err != nil {
		s.reply(550, "Remove directory failed")
		return
	}

	s.reply(250, "Directory removed")
}

// handleRnfr stores the rename source path.
func (s *session) handleRnfr(arg string) {
	s.renameFrom = s.resolvePath(arg)
	s.reply(350, "Ready for RNTO")
}

// handleRnto completes a rename operation.
func (s *session) handleRnto(arg string) {
	if s.renameFrom == "" {
		s.reply(503, "Send RNFR first")
		return
	}

	from := s.renameFrom
	to := s.resolvePath(arg)
	s.renameFrom = ""

	fromVault, fromPath := parsePath(from)
	toVault, toPath := parsePath(to)

	if fromVault == "" || toVault == "" {
		s.reply(550, "Invalid path")
		return
	}
	if fromVault != toVault {
		s.reply(553, "Cross-vault rename not supported")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	vaultID, err := s.resolveVault(ctx, fromVault)
	if err != nil {
		s.reply(550, "Vault not found")
		return
	}

	// Try as document first
	doc, err := s.dbClient.GetFileByPath(ctx, vaultID, fromPath)
	if err != nil {
		s.reply(550, "Rename failed")
		return
	}
	if doc != nil {
		if !isMarkdownFile(toPath) {
			s.reply(553, "Only markdown files (.md) are allowed")
			return
		}
		if _, err := s.docService.Move(ctx, vaultID, fromPath, toPath); err != nil {
			s.reply(550, "Rename failed")
			return
		}
		s.reply(250, "File renamed")
		return
	}

	// Try as folder
	folder, err := s.dbClient.GetFolderByPath(ctx, vaultID, fromPath)
	if err != nil {
		s.reply(550, "Rename failed")
		return
	}
	if folder != nil {
		if err := s.vaultSvc.MoveFolder(ctx, vaultID, fromPath, toPath); err != nil {
			s.reply(550, "Rename failed")
			return
		}
		s.reply(250, "Directory renamed")
		return
	}

	s.reply(550, "File or directory not found")
}

// handleSize returns the file size.
func (s *session) handleSize(arg string) {
	filePath := s.resolvePath(arg)
	vaultName, docPath := parsePath(filePath)
	if vaultName == "" || docPath == "/" {
		s.reply(550, "Not a file")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	vaultID, err := s.resolveVault(ctx, vaultName)
	if err != nil {
		s.reply(550, "Vault not found")
		return
	}

	meta, err := s.dbClient.GetFileMetaByPath(ctx, vaultID, docPath)
	if err != nil || meta == nil {
		s.reply(550, "File not found")
		return
	}

	s.reply(213, fmt.Sprintf("%d", meta.Size))
}

// handleMdtm returns the file modification time.
func (s *session) handleMdtm(arg string) {
	filePath := s.resolvePath(arg)
	vaultName, docPath := parsePath(filePath)
	if vaultName == "" || docPath == "/" {
		s.reply(550, "Not a file")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	vaultID, err := s.resolveVault(ctx, vaultName)
	if err != nil {
		s.reply(550, "Vault not found")
		return
	}

	meta, err := s.dbClient.GetFileMetaByPath(ctx, vaultID, docPath)
	if err != nil || meta == nil {
		s.reply(550, "File not found")
		return
	}

	s.reply(213, meta.UpdatedAt.UTC().Format("20060102150405"))
}

// resolvePath resolves a relative or absolute path against the cwd.
func (s *session) resolvePath(p string) string {
	if p == "" {
		return s.cwd
	}
	if !strings.HasPrefix(p, "/") {
		p = s.cwd + "/" + p
	}
	return path.Clean(p)
}

// parsePath splits an FTP path into vault name and document path.
// "/" → ("", "")
// "/myvault" → ("myvault", "/")
// "/myvault/notes/foo.md" → ("myvault", "/notes/foo.md")
func parsePath(p string) (vaultName, docPath string) {
	p = path.Clean(p)
	if p == "." || p == "/" || p == "" {
		return "", ""
	}

	p = strings.TrimPrefix(p, "/")
	parts := strings.SplitN(p, "/", 2)
	vaultName = parts[0]

	if len(parts) == 1 {
		return vaultName, "/"
	}

	return vaultName, "/" + parts[1]
}

// resolveVault looks up a vault by name and checks access.
func (s *session) resolveVault(ctx context.Context, vaultName string) (string, error) {
	id, err := auth.ResolveVault(ctx, *s.ac, s.vaultSvc, vaultName)
	if err != nil {
		s.logger.Debug("vault resolution failed", "vault", vaultName, "error", err)
		if errors.Is(err, os.ErrNotExist) {
			return "", os.ErrNotExist
		}
		return "", os.ErrPermission
	}
	return id, nil
}

// isMarkdownFile returns true if the file has a .md extension (case-insensitive).
func isMarkdownFile(name string) bool {
	return strings.EqualFold(path.Ext(name), ".md")
}
