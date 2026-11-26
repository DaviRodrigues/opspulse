//go:build e2e

package mongofake

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	opReply = int32(1)
	opQuery = int32(2004)
	opMsg   = int32(2013)

	msgFlagChecksumPresent = uint32(1 << 0)
	msgFlagMoreToCome      = uint32(1 << 1)
)

type indexDef struct {
	Name   string
	Keys   bson.D
	Unique bool
}

type collection struct {
	docs    []bson.D
	indexes []indexDef
	options bson.D
}

type database struct {
	collections map[string]*collection
}

type cursorState struct {
	ns   string
	docs []bson.D
	pos  int
}

// Server is a thread-safe, in-memory MongoDB Wire Protocol (OP_MSG / OP_QUERY) fake server.
type Server struct {
	listener  net.Listener
	mu        sync.RWMutex
	dbs       map[string]*database
	cursors   map[int64]*cursorState
	nextReqID atomic.Int32
	nextCurID atomic.Int64

	closed atomic.Bool
	connMu sync.Mutex
	conns  map[net.Conn]struct{}
	wg     sync.WaitGroup
}

// NewServer starts an in-memory MongoDB fake listening on 127.0.0.1:0.
func NewServer() (*Server, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("mongofake listen: %w", err)
	}

	s := &Server{
		listener: ln,
		dbs:      make(map[string]*database),
		cursors:  make(map[int64]*cursorState),
		conns:    make(map[net.Conn]struct{}),
	}
	s.nextCurID.Store(100)

	s.wg.Add(1)
	go s.acceptLoop()

	return s, nil
}

// URI returns the connection URI for go.mongodb.org/mongo-driver/v2.
func (s *Server) URI() string {
	return fmt.Sprintf("mongodb://%s/?directConnection=true", s.listener.Addr().String())
}

// Addr returns the host:port string of the listener.
func (s *Server) Addr() string {
	return s.listener.Addr().String()
}

// Close shuts down the fake server and waits for active connection goroutines.
func (s *Server) Close() error {
	if s.closed.Swap(true) {
		return nil
	}
	err := s.listener.Close()

	s.connMu.Lock()
	for c := range s.conns {
		_ = c.Close()
	}
	s.connMu.Unlock()

	s.wg.Wait()
	return err
}

// Count returns the number of documents in dbName.collName.
func (s *Server) Count(dbName, collName string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	db, ok := s.dbs[dbName]
	if !ok {
		return 0
	}
	coll, ok := db.collections[collName]
	if !ok {
		return 0
	}
	return len(coll.docs)
}

// Clear removes all documents from dbName.collName (preserving indexes).
func (s *Server) Clear(dbName, collName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	db, ok := s.dbs[dbName]
	if !ok {
		return
	}
	coll, ok := db.collections[collName]
	if !ok {
		return
	}
	coll.docs = nil
}

func (s *Server) acceptLoop() {
	defer s.wg.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if s.closed.Load() || errors.Is(err, net.ErrClosed) {
				return
			}
			return
		}
		s.connMu.Lock()
		if s.closed.Load() {
			s.connMu.Unlock()
			_ = conn.Close()
			return
		}
		s.conns[conn] = struct{}{}
		s.connMu.Unlock()

		s.wg.Add(1)
		go s.serveConn(conn)
	}
}

func (s *Server) serveConn(conn net.Conn) {
	defer s.wg.Done()
	defer func() {
		s.connMu.Lock()
		delete(s.conns, conn)
		s.connMu.Unlock()
		_ = conn.Close()
	}()

	var header [16]byte
	for {
		if _, err := io.ReadFull(conn, header[:]); err != nil {
			return
		}

		msgLen := int32(binary.LittleEndian.Uint32(header[0:4]))
		reqID := int32(binary.LittleEndian.Uint32(header[4:8]))
		opCode := int32(binary.LittleEndian.Uint32(header[12:16]))

		if msgLen < 16 || msgLen > 64*1024*1024 {
			return
		}

		payload := make([]byte, msgLen-16)
		if _, err := io.ReadFull(conn, payload); err != nil {
			return
		}

		switch opCode {
		case opMsg:
			replyDoc, needReply, err := s.handleOpMsg(payload)
			if err != nil {
				replyDoc = errorReply(2, "BadValue", err.Error())
				needReply = true
			}
			if needReply {
				if err := s.writeOpMsgReply(conn, reqID, replyDoc); err != nil {
					return
				}
			}
		case opQuery:
			replyDoc, err := s.handleOpQuery(payload)
			if err != nil {
				replyDoc = errorReply(2, "BadValue", err.Error())
			}
			if err := s.writeOpReply(conn, reqID, replyDoc); err != nil {
				return
			}
		default:
			replyDoc := errorReply(59, "CommandNotFound", fmt.Sprintf("unsupported opcode %d", opCode))
			if err := s.writeOpMsgReply(conn, reqID, replyDoc); err != nil {
				return
			}
		}
	}
}

func (s *Server) handleOpQuery(payload []byte) (bson.D, error) {
	if len(payload) < 4 {
		return nil, errors.New("short OP_QUERY")
	}
	pos := 4 // flags
	nulIdx := bytes.IndexByte(payload[pos:], 0)
	if nulIdx < 0 {
		return nil, errors.New("unterminated collection name")
	}
	fullColl := string(payload[pos : pos+nulIdx])
	pos += nulIdx + 1
	if pos+8 > len(payload) {
		return nil, errors.New("short OP_QUERY skip/return")
	}
	pos += 8 // numberToSkip, numberToReturn

	docLen, err := readBSONDocLen(payload[pos:])
	if err != nil {
		return nil, err
	}
	var cmd bson.D
	if err := bson.Unmarshal(payload[pos:pos+docLen], &cmd); err != nil {
		return nil, err
	}

	dbName := "admin"
	if dot := strings.IndexByte(fullColl, '.'); dot > 0 {
		dbName = fullColl[:dot]
	}
	return s.dispatchCommand(dbName, cmd), nil
}

func (s *Server) handleOpMsg(payload []byte) (bson.D, bool, error) {
	if len(payload) < 5 {
		return nil, true, errors.New("short OP_MSG")
	}
	flags := binary.LittleEndian.Uint32(payload[0:4])
	needReply := (flags & msgFlagMoreToCome) == 0

	body := payload[4:]
	if (flags & msgFlagChecksumPresent) != 0 {
		if len(body) < 4 {
			return nil, needReply, errors.New("short OP_MSG checksum")
		}
		body = body[:len(body)-4]
	}

	var cmd bson.D
	pos := 0
	for pos < len(body) {
		kind := body[pos]
		pos++
		switch kind {
		case 0:
			docLen, err := readBSONDocLen(body[pos:])
			if err != nil {
				return nil, needReply, err
			}
			var doc bson.D
			if err := bson.Unmarshal(body[pos:pos+docLen], &doc); err != nil {
				return nil, needReply, err
			}
			pos += docLen
			cmd = append(cmd, doc...)
		case 1:
			if pos+4 > len(body) {
				return nil, needReply, errors.New("short OP_MSG section kind 1")
			}
			secSize := int(int32(binary.LittleEndian.Uint32(body[pos : pos+4])))
			if secSize < 5 || pos+secSize > len(body) {
				return nil, needReply, errors.New("invalid OP_MSG section kind 1 size")
			}
			secEnd := pos + secSize
			pos += 4

			nulIdx := bytes.IndexByte(body[pos:secEnd], 0)
			if nulIdx < 0 {
				return nil, needReply, errors.New("unterminated OP_MSG sequence identifier")
			}
			ident := string(body[pos : pos+nulIdx])
			pos += nulIdx + 1

			var seq bson.A
			for pos < secEnd {
				docLen, err := readBSONDocLen(body[pos:secEnd])
				if err != nil {
					return nil, needReply, err
				}
				var doc bson.D
				if err := bson.Unmarshal(body[pos:pos+docLen], &doc); err != nil {
					return nil, needReply, err
				}
				pos += docLen
				seq = append(seq, doc)
			}
			cmd = docSet(cmd, ident, seq)
		default:
			return nil, needReply, fmt.Errorf("unknown OP_MSG section kind %d", kind)
		}
	}

	dbName, _ := docGetString(cmd, "$db")
	if dbName == "" {
		dbName = "test"
	}
	return s.dispatchCommand(dbName, cmd), needReply, nil
}

func (s *Server) writeOpMsgReply(conn net.Conn, responseTo int32, doc bson.D) error {
	rawDoc, err := bson.Marshal(doc)
	if err != nil {
		return err
	}
	totalLen := 16 + 4 + 1 + len(rawDoc)
	buf := make([]byte, totalLen)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(totalLen))
	binary.LittleEndian.PutUint32(buf[4:8], uint32(s.nextReqID.Add(1)))
	binary.LittleEndian.PutUint32(buf[8:12], uint32(responseTo))
	binary.LittleEndian.PutUint32(buf[12:16], uint32(opMsg))
	binary.LittleEndian.PutUint32(buf[16:20], 0) // flagBits
	buf[20] = 0                                  // kind 0
	copy(buf[21:], rawDoc)
	_, err = conn.Write(buf)
	return err
}

func (s *Server) writeOpReply(conn net.Conn, responseTo int32, doc bson.D) error {
	rawDoc, err := bson.Marshal(doc)
	if err != nil {
		return err
	}
	totalLen := 16 + 20 + len(rawDoc)
	buf := make([]byte, totalLen)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(totalLen))
	binary.LittleEndian.PutUint32(buf[4:8], uint32(s.nextReqID.Add(1)))
	binary.LittleEndian.PutUint32(buf[8:12], uint32(responseTo))
	binary.LittleEndian.PutUint32(buf[12:16], uint32(opReply))
	binary.LittleEndian.PutUint32(buf[16:20], 0) // responseFlags
	binary.LittleEndian.PutUint64(buf[20:28], 0) // cursorID
	binary.LittleEndian.PutUint32(buf[28:32], 0) // startingFrom
	binary.LittleEndian.PutUint32(buf[32:36], 1) // numberReturned
	copy(buf[36:], rawDoc)
	_, err = conn.Write(buf)
	return err
}

func readBSONDocLen(b []byte) (int, error) {
	if len(b) < 4 {
		return 0, errors.New("short BSON length")
	}
	n := int(int32(binary.LittleEndian.Uint32(b[:4])))
	if n < 5 || n > len(b) {
		return 0, fmt.Errorf("invalid BSON document length %d (available %d)", n, len(b))
	}
	return n, nil
}

func (s *Server) dispatchCommand(dbName string, cmd bson.D) bson.D {
	if len(cmd) == 0 {
		return errorReply(2, "BadValue", "empty command")
	}

	cmdName := cmd[0].Key
	switch cmdName {
	case "hello", "ismaster", "isMaster":
		return bson.D{
			{Key: "isWritablePrimary", Value: true},
			{Key: "ismaster", Value: true},
			{Key: "helloOk", Value: true},
			{Key: "maxBsonObjectSize", Value: int32(16777216)},
			{Key: "maxMessageSizeBytes", Value: int32(48000000)},
			{Key: "maxWriteBatchSize", Value: int32(100000)},
			{Key: "localTime", Value: bson.NewDateTimeFromTime(time.Now().UTC())},
			{Key: "logicalSessionTimeoutMinutes", Value: int32(30)},
			{Key: "minWireVersion", Value: int32(0)},
			{Key: "maxWireVersion", Value: int32(21)},
			{Key: "readOnly", Value: false},
			{Key: "ok", Value: float64(1)},
		}
	case "ping":
		return bson.D{{Key: "ok", Value: float64(1)}}
	case "buildInfo", "buildinfo":
		return bson.D{
			{Key: "version", Value: "7.0.0"},
			{Key: "versionArray", Value: bson.A{int32(7), int32(0), int32(0), int32(0)}},
			{Key: "ok", Value: float64(1)},
		}
	case "endSessions":
		return bson.D{{Key: "ok", Value: float64(1)}}
	case "create":
		return s.cmdCreate(dbName, cmd)
	case "listCollections":
		return s.cmdListCollections(dbName, cmd)
	case "createIndexes":
		return s.cmdCreateIndexes(dbName, cmd)
	case "listIndexes":
		return s.cmdListIndexes(dbName, cmd)
	case "insert":
		return s.cmdInsert(dbName, cmd)
	case "find":
		return s.cmdFind(dbName, cmd)
	case "getMore":
		return s.cmdGetMore(dbName, cmd)
	case "killCursors":
		return s.cmdKillCursors(cmd)
	case "update":
		return s.cmdUpdate(dbName, cmd)
	case "findAndModify":
		return s.cmdFindAndModify(dbName, cmd)
	case "delete":
		return s.cmdDelete(dbName, cmd)
	case "count":
		return s.cmdCount(dbName, cmd)
	case "aggregate":
		return s.cmdAggregate(dbName, cmd)
	case "drop":
		return s.cmdDrop(dbName, cmd)
	default:
		return errorReply(59, "CommandNotFound", fmt.Sprintf("no such command: '%s'", cmdName))
	}
}

type cmdError struct {
	code     int32
	codeName string
	msg      string
}

func (e *cmdError) Error() string {
	return e.msg
}

func errorReply(code int32, codeName, msg string) bson.D {
	return bson.D{
		{Key: "ok", Value: float64(0)},
		{Key: "errmsg", Value: msg},
		{Key: "code", Value: code},
		{Key: "codeName", Value: codeName},
	}
}

func errorReplyFromErr(err error) bson.D {
	var ce *cmdError
	if errors.As(err, &ce) {
		return errorReply(ce.code, ce.codeName, ce.msg)
	}
	return errorReply(2, "BadValue", err.Error())
}

func (s *Server) getOrCreateCollLocked(dbName, collName string) *collection {
	db, ok := s.dbs[dbName]
	if !ok {
		db = &database{collections: make(map[string]*collection)}
		s.dbs[dbName] = db
	}
	coll, ok := db.collections[collName]
	if !ok {
		coll = &collection{
			indexes: []indexDef{
				{Name: "_id_", Keys: bson.D{{Key: "_id", Value: int32(1)}}, Unique: true},
			},
		}
		db.collections[collName] = coll
	}
	return coll
}

func (s *Server) getCollLocked(dbName, collName string) *collection {
	db, ok := s.dbs[dbName]
	if !ok {
		return nil
	}
	return db.collections[collName]
}

func (s *Server) cmdCreate(dbName string, cmd bson.D) bson.D {
	collName, ok := docGetString(cmd, "create")
	if !ok || collName == "" {
		return errorReply(2, "BadValue", "missing collection name")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.getCollLocked(dbName, collName) != nil {
		return errorReply(48, "NamespaceExists", fmt.Sprintf("Collection already exists. NS: %s.%s", dbName, collName))
	}
	coll := s.getOrCreateCollLocked(dbName, collName)
	var opts bson.D
	for _, elem := range cmd {
		if elem.Key == "timeseries" || elem.Key == "expireAfterSeconds" {
			opts = append(opts, elem)
		}
	}
	if len(opts) > 0 {
		coll.options = opts
	}
	return bson.D{{Key: "ok", Value: float64(1)}}
}

func (s *Server) cmdDrop(dbName string, cmd bson.D) bson.D {
	collName, _ := docGetString(cmd, "drop")
	s.mu.Lock()
	defer s.mu.Unlock()
	if db, ok := s.dbs[dbName]; ok {
		delete(db.collections, collName)
	}
	return bson.D{{Key: "ns", Value: dbName + "." + collName}, {Key: "ok", Value: float64(1)}}
}

func (s *Server) cmdListCollections(dbName string, cmd bson.D) bson.D {
	filter, _ := docGetDoc(cmd, "filter")
	if err := validateFilter(filter); err != nil {
		return errorReplyFromErr(err)
	}

	s.mu.RLock()
	var batch bson.A
	if db, ok := s.dbs[dbName]; ok {
		for name, coll := range db.collections {
			info := bson.D{
				{Key: "name", Value: name},
				{Key: "type", Value: "collection"},
				{Key: "options", Value: cloneDoc(coll.options)},
			}
			if len(filter) == 0 {
				batch = append(batch, info)
			} else {
				matched, err := matchDoc(info, filter)
				if err != nil {
					s.mu.RUnlock()
					return errorReplyFromErr(err)
				}
				if matched {
					batch = append(batch, info)
				}
			}
		}
	}
	s.mu.RUnlock()

	if batch == nil {
		batch = bson.A{}
	}
	return bson.D{
		{Key: "cursor", Value: bson.D{
			{Key: "id", Value: int64(0)},
			{Key: "ns", Value: dbName + ".$cmd.listCollections"},
			{Key: "firstBatch", Value: batch},
		}},
		{Key: "ok", Value: float64(1)},
	}
}

func (s *Server) cmdListIndexes(dbName string, cmd bson.D) bson.D {
	collName, ok := docGetString(cmd, "listIndexes")
	if !ok || collName == "" {
		return errorReply(2, "BadValue", "missing collection name")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	batch := bson.A{}
	if coll := s.getCollLocked(dbName, collName); coll != nil {
		for _, idx := range coll.indexes {
			idxDoc := bson.D{
				{Key: "v", Value: int32(2)},
				{Key: "key", Value: cloneDoc(idx.Keys)},
				{Key: "name", Value: idx.Name},
			}
			if idx.Unique {
				idxDoc = append(idxDoc, bson.E{Key: "unique", Value: true})
			}
			batch = append(batch, idxDoc)
		}
	}

	return bson.D{
		{Key: "cursor", Value: bson.D{
			{Key: "id", Value: int64(0)},
			{Key: "ns", Value: dbName + "." + collName},
			{Key: "firstBatch", Value: batch},
		}},
		{Key: "ok", Value: float64(1)},
	}
}

func (s *Server) cmdCreateIndexes(dbName string, cmd bson.D) bson.D {
	collName, ok := docGetString(cmd, "createIndexes")
	if !ok || collName == "" {
		return errorReply(2, "BadValue", "missing collection name")
	}
	rawIdxs, _ := docGetArray(cmd, "indexes")

	s.mu.Lock()
	defer s.mu.Unlock()
	coll := s.getOrCreateCollLocked(dbName, collName)
	before := int32(len(coll.indexes))

	for _, item := range rawIdxs {
		idxDoc, ok := asDoc(item)
		if !ok {
			continue
		}
		keys, _ := docGetDoc(idxDoc, "key")
		name, _ := docGetString(idxDoc, "name")
		unique, _ := docGetBool(idxDoc, "unique")
		if name == "" {
			var parts []string
			for _, k := range keys {
				parts = append(parts, fmt.Sprintf("%s_%v", k.Key, k.Value))
			}
			name = strings.Join(parts, "_")
		}

		exists := false
		for _, existing := range coll.indexes {
			if existing.Name == name || docsEqual(existing.Keys, keys) {
				exists = true
				break
			}
		}
		if !exists {
			if unique {
				// Validate existing docs don't violate uniqueness
				for i := 0; i < len(coll.docs); i++ {
					for j := i + 1; j < len(coll.docs); j++ {
						if indexKeysCollide(coll.docs[i], coll.docs[j], keys) {
							return errorReply(11000, "DuplicateKey", fmt.Sprintf("E11000 duplicate key error collection: %s.%s index: %s", dbName, collName, name))
						}
					}
				}
			}
			coll.indexes = append(coll.indexes, indexDef{
				Name:   name,
				Keys:   cloneDoc(keys),
				Unique: unique,
			})
		}
	}

	return bson.D{
		{Key: "createdCollectionAutomatically", Value: false},
		{Key: "numIndexesBefore", Value: before},
		{Key: "numIndexesAfter", Value: int32(len(coll.indexes))},
		{Key: "ok", Value: float64(1)},
	}
}

func (s *Server) cmdInsert(dbName string, cmd bson.D) bson.D {
	collName, ok := docGetString(cmd, "insert")
	if !ok || collName == "" {
		return errorReply(2, "BadValue", "missing collection name")
	}
	rawDocs, _ := docGetArray(cmd, "documents")
	ordered := true
	if v, ok := docGetBool(cmd, "ordered"); ok {
		ordered = v
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	coll := s.getOrCreateCollLocked(dbName, collName)

	var inserted int32
	var writeErrors bson.A

	for i, raw := range rawDocs {
		doc, ok := asDoc(raw)
		if !ok {
			writeErrors = append(writeErrors, bson.D{
				{Key: "index", Value: int32(i)},
				{Key: "code", Value: int32(2)},
				{Key: "errmsg", Value: "document is not a BSON object"},
			})
			if ordered {
				break
			}
			continue
		}

		cloned := cloneDoc(doc)
		if _, hasID := docGet(cloned, "_id"); !hasID {
			cloned = append(bson.D{{Key: "_id", Value: bson.NewObjectID()}}, cloned...)
		}

		if idxName, dup := checkUniqueConflict(coll, cloned, -1); dup {
			writeErrors = append(writeErrors, bson.D{
				{Key: "index", Value: int32(i)},
				{Key: "code", Value: int32(11000)},
				{Key: "errmsg", Value: fmt.Sprintf("E11000 duplicate key error collection: %s.%s index: %s", dbName, collName, idxName)},
			})
			if ordered {
				break
			}
			continue
		}

		coll.docs = append(coll.docs, cloned)
		inserted++
	}

	reply := bson.D{
		{Key: "n", Value: inserted},
		{Key: "ok", Value: float64(1)},
	}
	if len(writeErrors) > 0 {
		reply = append(reply, bson.E{Key: "writeErrors", Value: writeErrors})
	}
	return reply
}

func (s *Server) cmdFind(dbName string, cmd bson.D) bson.D {
	collName, ok := docGetString(cmd, "find")
	if !ok || collName == "" {
		return errorReply(2, "BadValue", "missing collection name")
	}
	filter, _ := docGetDoc(cmd, "filter")
	if err := validateFilter(filter); err != nil {
		return errorReplyFromErr(err)
	}
	sortSpec, _ := docGetDoc(cmd, "sort")
	projSpec, _ := docGetDoc(cmd, "projection")
	skip, _ := docGetInt64(cmd, "skip")
	limit, _ := docGetInt64(cmd, "limit")
	batchSize, _ := docGetInt64(cmd, "batchSize")
	singleBatch, _ := docGetBool(cmd, "singleBatch")

	s.mu.Lock()
	defer s.mu.Unlock()

	var matched []bson.D
	if coll := s.getCollLocked(dbName, collName); coll != nil {
		for _, d := range coll.docs {
			m, err := matchDoc(d, filter)
			if err != nil {
				return errorReplyFromErr(err)
			}
			if m {
				matched = append(matched, cloneDoc(d))
			}
		}
	}

	if len(sortSpec) > 0 {
		sortDocs(matched, sortSpec)
	}
	matched = applySkipLimit(matched, skip, limit)
	if len(projSpec) > 0 {
		for i := range matched {
			matched[i] = applyProjection(matched[i], projSpec)
		}
	}

	ns := dbName + "." + collName
	return s.buildCursorReplyLocked(ns, matched, batchSize, singleBatch, "firstBatch")
}

func (s *Server) cmdGetMore(dbName string, cmd bson.D) bson.D {
	cursorID, ok := docGetInt64(cmd, "getMore")
	if !ok {
		return errorReply(2, "BadValue", "missing cursor id")
	}
	batchSize, _ := docGetInt64(cmd, "batchSize")

	s.mu.Lock()
	defer s.mu.Unlock()

	cur, exists := s.cursors[cursorID]
	if !exists {
		return errorReply(43, "CursorNotFound", fmt.Sprintf("cursor id %d not found", cursorID))
	}

	remaining := cur.docs[cur.pos:]
	n := len(remaining)
	if batchSize > 0 && int(batchSize) < n {
		n = int(batchSize)
	}
	batch := make(bson.A, 0, n)
	for i := 0; i < n; i++ {
		batch = append(batch, remaining[i])
	}
	cur.pos += n

	nextID := cursorID
	if cur.pos >= len(cur.docs) {
		delete(s.cursors, cursorID)
		nextID = 0
	}

	return bson.D{
		{Key: "cursor", Value: bson.D{
			{Key: "id", Value: nextID},
			{Key: "ns", Value: cur.ns},
			{Key: "nextBatch", Value: batch},
		}},
		{Key: "ok", Value: float64(1)},
	}
}

func (s *Server) cmdKillCursors(cmd bson.D) bson.D {
	ids, _ := docGetArray(cmd, "cursors")

	s.mu.Lock()
	defer s.mu.Unlock()

	killed := bson.A{}
	notFound := bson.A{}
	for _, raw := range ids {
		id, ok := asInt64(raw)
		if !ok {
			continue
		}
		if _, exists := s.cursors[id]; exists {
			delete(s.cursors, id)
			killed = append(killed, id)
		} else {
			notFound = append(notFound, id)
		}
	}

	return bson.D{
		{Key: "cursorsKilled", Value: killed},
		{Key: "cursorsNotFound", Value: notFound},
		{Key: "cursorsAlive", Value: bson.A{}},
		{Key: "cursorsUnknown", Value: bson.A{}},
		{Key: "ok", Value: float64(1)},
	}
}

func (s *Server) buildCursorReplyLocked(ns string, docs []bson.D, batchSize int64, singleBatch bool, batchKey string) bson.D {
	split := len(docs)
	var cursorID int64
	if !singleBatch && batchSize > 0 && int(batchSize) < len(docs) {
		split = int(batchSize)
		cursorID = s.nextCurID.Add(1)
		s.cursors[cursorID] = &cursorState{
			ns:   ns,
			docs: docs,
			pos:  split,
		}
	}

	batch := make(bson.A, 0, split)
	for i := 0; i < split; i++ {
		batch = append(batch, docs[i])
	}

	return bson.D{
		{Key: "cursor", Value: bson.D{
			{Key: "id", Value: cursorID},
			{Key: "ns", Value: ns},
			{Key: batchKey, Value: batch},
		}},
		{Key: "ok", Value: float64(1)},
	}
}

func (s *Server) cmdUpdate(dbName string, cmd bson.D) bson.D {
	collName, ok := docGetString(cmd, "update")
	if !ok || collName == "" {
		return errorReply(2, "BadValue", "missing collection name")
	}
	rawUpdates, _ := docGetArray(cmd, "updates")
	ordered := true
	if v, ok := docGetBool(cmd, "ordered"); ok {
		ordered = v
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	coll := s.getOrCreateCollLocked(dbName, collName)

	var matchedTotal int32
	var modifiedTotal int32
	var upserted bson.A
	var writeErrors bson.A

	for i, rawUpd := range rawUpdates {
		uDoc, ok := asDoc(rawUpd)
		if !ok {
			continue
		}
		qFilter, _ := docGetDoc(uDoc, "q")
		if err := validateFilter(qFilter); err != nil {
			return errorReplyFromErr(err)
		}
		uSpec, _ := docGetDoc(uDoc, "u")
		upsert, _ := docGetBool(uDoc, "upsert")
		multi, _ := docGetBool(uDoc, "multi")

		matchedAny := false
		failed := false
		for docIdx := 0; docIdx < len(coll.docs); docIdx++ {
			m, err := matchDoc(coll.docs[docIdx], qFilter)
			if err != nil {
				return errorReplyFromErr(err)
			}
			if !m {
				continue
			}
			matchedAny = true
			updated, err := applyUpdate(coll.docs[docIdx], uSpec, false)
			if err != nil {
				writeErrors = append(writeErrors, bson.D{
					{Key: "index", Value: int32(i)},
					{Key: "code", Value: int32(2)},
					{Key: "errmsg", Value: err.Error()},
				})
				failed = true
				break
			}
			if idxName, dup := checkUniqueConflict(coll, updated, docIdx); dup {
				writeErrors = append(writeErrors, bson.D{
					{Key: "index", Value: int32(i)},
					{Key: "code", Value: int32(11000)},
					{Key: "errmsg", Value: fmt.Sprintf("E11000 duplicate key error collection: %s.%s index: %s", dbName, collName, idxName)},
				})
				failed = true
				break
			}
			coll.docs[docIdx] = updated
			matchedTotal++
			modifiedTotal++
			if !multi {
				break
			}
		}

		if failed && ordered {
			break
		}

		if !matchedAny && !failed && upsert {
			base := buildUpsertBase(qFilter)
			newDoc, err := applyUpdate(base, uSpec, true)
			if err != nil {
				writeErrors = append(writeErrors, bson.D{
					{Key: "index", Value: int32(i)},
					{Key: "code", Value: int32(2)},
					{Key: "errmsg", Value: err.Error()},
				})
				if ordered {
					break
				}
				continue
			}
			if _, hasID := docGet(newDoc, "_id"); !hasID {
				newDoc = append(bson.D{{Key: "_id", Value: bson.NewObjectID()}}, newDoc...)
			}
			if idxName, dup := checkUniqueConflict(coll, newDoc, -1); dup {
				writeErrors = append(writeErrors, bson.D{
					{Key: "index", Value: int32(i)},
					{Key: "code", Value: int32(11000)},
					{Key: "errmsg", Value: fmt.Sprintf("E11000 duplicate key error collection: %s.%s index: %s", dbName, collName, idxName)},
				})
				if ordered {
					break
				}
				continue
			}
			coll.docs = append(coll.docs, newDoc)
			matchedTotal++
			idVal, _ := docGet(newDoc, "_id")
			upserted = append(upserted, bson.D{
				{Key: "index", Value: int32(i)},
				{Key: "_id", Value: idVal},
			})
		}
	}

	reply := bson.D{
		{Key: "n", Value: matchedTotal},
		{Key: "nModified", Value: modifiedTotal},
		{Key: "ok", Value: float64(1)},
	}
	if len(upserted) > 0 {
		reply = append(reply, bson.E{Key: "upserted", Value: upserted})
	}
	if len(writeErrors) > 0 {
		reply = append(reply, bson.E{Key: "writeErrors", Value: writeErrors})
	}
	return reply
}

func (s *Server) cmdFindAndModify(dbName string, cmd bson.D) bson.D {
	collName, ok := docGetString(cmd, "findAndModify")
	if !ok || collName == "" {
		return errorReply(2, "BadValue", "missing collection name")
	}
	query, _ := docGetDoc(cmd, "query")
	if err := validateFilter(query); err != nil {
		return errorReplyFromErr(err)
	}
	sortSpec, _ := docGetDoc(cmd, "sort")
	updateSpec, hasUpdate := docGetDoc(cmd, "update")
	remove, _ := docGetBool(cmd, "remove")
	returnNew, _ := docGetBool(cmd, "new")
	upsert, _ := docGetBool(cmd, "upsert")
	fields, _ := docGetDoc(cmd, "fields")

	s.mu.Lock()
	defer s.mu.Unlock()
	coll := s.getOrCreateCollLocked(dbName, collName)

	type indexedDoc struct {
		idx int
		doc bson.D
	}
	var candidates []indexedDoc
	for i, d := range coll.docs {
		m, err := matchDoc(d, query)
		if err != nil {
			return errorReplyFromErr(err)
		}
		if m {
			candidates = append(candidates, indexedDoc{idx: i, doc: d})
		}
	}

	if len(candidates) > 1 && len(sortSpec) > 0 {
		slices.SortStableFunc(candidates, func(a, b indexedDoc) int {
			return compareDocsBySort(a.doc, b.doc, sortSpec)
		})
	}

	if len(candidates) == 0 {
		if remove || !hasUpdate || !upsert {
			return bson.D{
				{Key: "lastErrorObject", Value: bson.D{{Key: "n", Value: int32(0)}}},
				{Key: "value", Value: nil},
				{Key: "ok", Value: float64(1)},
			}
		}
		base := buildUpsertBase(query)
		newDoc, err := applyUpdate(base, updateSpec, true)
		if err != nil {
			return errorReply(2, "BadValue", err.Error())
		}
		if _, hasID := docGet(newDoc, "_id"); !hasID {
			newDoc = append(bson.D{{Key: "_id", Value: bson.NewObjectID()}}, newDoc...)
		}
		if idxName, dup := checkUniqueConflict(coll, newDoc, -1); dup {
			return errorReply(11000, "DuplicateKey", fmt.Sprintf("E11000 duplicate key error collection: %s.%s index: %s", dbName, collName, idxName))
		}
		coll.docs = append(coll.docs, newDoc)
		idVal, _ := docGet(newDoc, "_id")
		var retVal any = nil
		if returnNew {
			out := cloneDoc(newDoc)
			if len(fields) > 0 {
				out = applyProjection(out, fields)
			}
			retVal = out
		}
		return bson.D{
			{Key: "lastErrorObject", Value: bson.D{
				{Key: "n", Value: int32(1)},
				{Key: "updatedExisting", Value: false},
				{Key: "upserted", Value: idVal},
			}},
			{Key: "value", Value: retVal},
			{Key: "ok", Value: float64(1)},
		}
	}

	target := candidates[0]
	oldDoc := cloneDoc(target.doc)

	if remove {
		coll.docs = append(coll.docs[:target.idx], coll.docs[target.idx+1:]...)
		if len(fields) > 0 {
			oldDoc = applyProjection(oldDoc, fields)
		}
		return bson.D{
			{Key: "lastErrorObject", Value: bson.D{{Key: "n", Value: int32(1)}}},
			{Key: "value", Value: oldDoc},
			{Key: "ok", Value: float64(1)},
		}
	}

	updated, err := applyUpdate(oldDoc, updateSpec, false)
	if err != nil {
		return errorReply(2, "BadValue", err.Error())
	}
	if idxName, dup := checkUniqueConflict(coll, updated, target.idx); dup {
		return errorReply(11000, "DuplicateKey", fmt.Sprintf("E11000 duplicate key error collection: %s.%s index: %s", dbName, collName, idxName))
	}
	coll.docs[target.idx] = updated

	retDoc := oldDoc
	if returnNew {
		retDoc = cloneDoc(updated)
	}
	if len(fields) > 0 {
		retDoc = applyProjection(retDoc, fields)
	}
	return bson.D{
		{Key: "lastErrorObject", Value: bson.D{
			{Key: "n", Value: int32(1)},
			{Key: "updatedExisting", Value: true},
		}},
		{Key: "value", Value: retDoc},
		{Key: "ok", Value: float64(1)},
	}
}

func (s *Server) cmdDelete(dbName string, cmd bson.D) bson.D {
	collName, ok := docGetString(cmd, "delete")
	if !ok || collName == "" {
		return errorReply(2, "BadValue", "missing collection name")
	}
	rawDeletes, _ := docGetArray(cmd, "deletes")

	s.mu.Lock()
	defer s.mu.Unlock()
	coll := s.getCollLocked(dbName, collName)
	if coll == nil {
		return bson.D{{Key: "n", Value: int32(0)}, {Key: "ok", Value: float64(1)}}
	}

	var deleted int32
	for _, rawDel := range rawDeletes {
		dDoc, ok := asDoc(rawDel)
		if !ok {
			continue
		}
		qFilter, _ := docGetDoc(dDoc, "q")
		if err := validateFilter(qFilter); err != nil {
			return errorReplyFromErr(err)
		}
		lim, _ := docGetInt64(dDoc, "limit")

		kept := make([]bson.D, 0, len(coll.docs))
		var removedThis int64
		for _, doc := range coll.docs {
			var matched bool
			if lim == 0 || removedThis < lim {
				var err error
				matched, err = matchDoc(doc, qFilter)
				if err != nil {
					return errorReplyFromErr(err)
				}
			}
			if matched {
				removedThis++
				deleted++
				continue
			}
			kept = append(kept, doc)
		}
		coll.docs = kept
	}

	return bson.D{{Key: "n", Value: deleted}, {Key: "ok", Value: float64(1)}}
}

func (s *Server) cmdCount(dbName string, cmd bson.D) bson.D {
	collName, ok := docGetString(cmd, "count")
	if !ok || collName == "" {
		return errorReply(2, "BadValue", "missing collection name")
	}
	query, _ := docGetDoc(cmd, "query")
	if err := validateFilter(query); err != nil {
		return errorReplyFromErr(err)
	}
	skip, _ := docGetInt64(cmd, "skip")
	limit, _ := docGetInt64(cmd, "limit")

	s.mu.RLock()
	var matched []bson.D
	if coll := s.getCollLocked(dbName, collName); coll != nil {
		for _, d := range coll.docs {
			m, err := matchDoc(d, query)
			if err != nil {
				s.mu.RUnlock()
				return errorReplyFromErr(err)
			}
			if m {
				matched = append(matched, d)
			}
		}
	}
	s.mu.RUnlock()

	matched = applySkipLimit(matched, skip, limit)
	return bson.D{{Key: "n", Value: int32(len(matched))}, {Key: "ok", Value: float64(1)}}
}

func (s *Server) cmdAggregate(dbName string, cmd bson.D) bson.D {
	collName, ok := docGetString(cmd, "aggregate")
	if !ok || collName == "" {
		return errorReply(2, "BadValue", "missing collection name")
	}
	pipeline, _ := docGetArray(cmd, "pipeline")
	var batchSize int64
	if curDoc, ok := docGetDoc(cmd, "cursor"); ok {
		batchSize, _ = docGetInt64(curDoc, "batchSize")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var docs []bson.D
	if coll := s.getCollLocked(dbName, collName); coll != nil {
		docs = make([]bson.D, len(coll.docs))
		for i, d := range coll.docs {
			docs[i] = cloneDoc(d)
		}
	}

	for _, rawStage := range pipeline {
		stage, ok := asDoc(rawStage)
		if !ok || len(stage) == 0 {
			continue
		}
		stageName := stage[0].Key
		stageVal := stage[0].Value

		switch stageName {
		case "$match":
			filter, _ := asDoc(stageVal)
			if err := validateFilter(filter); err != nil {
				return errorReplyFromErr(err)
			}
			var filtered []bson.D
			for _, d := range docs {
				m, err := matchDoc(d, filter)
				if err != nil {
					return errorReplyFromErr(err)
				}
				if m {
					filtered = append(filtered, d)
				}
			}
			docs = filtered
		case "$sort":
			sortSpec, _ := asDoc(stageVal)
			sortDocs(docs, sortSpec)
		case "$skip":
			n, _ := asInt64(stageVal)
			docs = applySkipLimit(docs, n, 0)
		case "$limit":
			n, _ := asInt64(stageVal)
			docs = applySkipLimit(docs, 0, n)
		case "$count":
			fieldName, _ := asString(stageVal)
			if fieldName == "" {
				fieldName = "count"
			}
			if len(docs) == 0 {
				docs = nil
			} else {
				docs = []bson.D{{{Key: fieldName, Value: int32(len(docs))}}}
			}
		case "$project":
			projSpec, _ := asDoc(stageVal)
			for i := range docs {
				docs[i] = applyProjection(docs[i], projSpec)
			}
		case "$group":
			groupSpec, _ := asDoc(stageVal)
			docs = applyGroupStage(docs, groupSpec)
		default:
			return errorReply(40324, "Location40324", fmt.Sprintf("unrecognized pipeline stage name: '%s'", stageName))
		}
	}

	return s.buildCursorReplyLocked(dbName+"."+collName, docs, batchSize, false, "firstBatch")
}

func applyGroupStage(docs []bson.D, spec bson.D) []bson.D {
	idExpr, _ := docGet(spec, "_id")

	type groupBucket struct {
		keyVal any
		docs   []bson.D
	}
	var buckets []groupBucket

	findOrAddBucket := func(k any) *groupBucket {
		for i := range buckets {
			if valuesEqual(buckets[i].keyVal, k) {
				return &buckets[i]
			}
		}
		buckets = append(buckets, groupBucket{keyVal: k})
		return &buckets[len(buckets)-1]
	}

	for _, d := range docs {
		key := evalGroupExpr(d, idExpr)
		b := findOrAddBucket(key)
		b.docs = append(b.docs, d)
	}

	out := make([]bson.D, 0, len(buckets))
	for _, b := range buckets {
		res := bson.D{{Key: "_id", Value: b.keyVal}}
		for _, elem := range spec {
			if elem.Key == "_id" {
				continue
			}
			accDoc, ok := asDoc(elem.Value)
			if !ok || len(accDoc) == 0 {
				continue
			}
			op := accDoc[0].Key
			arg := accDoc[0].Value
			switch op {
			case "$sum":
				var intSum int64
				var floatSum float64
				allInt := true
				for _, d := range b.docs {
					v := evalGroupExpr(d, arg)
					if n, ok := asInt64Strict(v); ok {
						if allInt {
							intSum += n
						} else {
							floatSum += float64(n)
						}
					} else if f, ok := asFloat64(v); ok {
						if allInt {
							floatSum = float64(intSum)
							allInt = false
						}
						floatSum += f
					}
				}
				if allInt {
					res = append(res, bson.E{Key: elem.Key, Value: intSum})
				} else {
					res = append(res, bson.E{Key: elem.Key, Value: floatSum})
				}
			case "$count":
				res = append(res, bson.E{Key: elem.Key, Value: int64(len(b.docs))})
			case "$avg":
				var sum float64
				var cnt int
				for _, d := range b.docs {
					v := evalGroupExpr(d, arg)
					if f, ok := asNumericFloat(v); ok {
						sum += f
						cnt++
					}
				}
				if cnt == 0 {
					res = append(res, bson.E{Key: elem.Key, Value: nil})
				} else {
					res = append(res, bson.E{Key: elem.Key, Value: sum / float64(cnt)})
				}
			case "$min":
				var minVal any
				for _, d := range b.docs {
					v := evalGroupExpr(d, arg)
					if v == nil {
						continue
					}
					if minVal == nil || compareBSONValues(v, minVal) < 0 {
						minVal = v
					}
				}
				res = append(res, bson.E{Key: elem.Key, Value: minVal})
			case "$max":
				var maxVal any
				for _, d := range b.docs {
					v := evalGroupExpr(d, arg)
					if v == nil {
						continue
					}
					if maxVal == nil || compareBSONValues(v, maxVal) > 0 {
						maxVal = v
					}
				}
				res = append(res, bson.E{Key: elem.Key, Value: maxVal})
			}
		}
		out = append(out, res)
	}
	return out
}

func evalGroupExpr(doc bson.D, expr any) any {
	if s, ok := expr.(string); ok && strings.HasPrefix(s, "$") {
		v, _ := docGetPath(doc, s[1:])
		return v
	}
	return expr
}

// --- Filtering & Matching ---

func validateFilter(filter bson.D) error {
	for _, elem := range filter {
		switch elem.Key {
		case "$and", "$or", "$nor":
			arr, ok := asArray(elem.Value)
			if !ok {
				continue
			}
			for _, item := range arr {
				if sub, ok := asDoc(item); ok {
					if err := validateFilter(sub); err != nil {
						return err
					}
				}
			}
		default:
			if err := validateFieldCond(elem.Value); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateFieldCond(cond any) error {
	if opDoc, ok := asDoc(cond); ok && isOperatorDoc(opDoc) {
		var regexPattern string
		var regexOptions string
		hasRegex := false
		for _, op := range opDoc {
			switch op.Key {
			case "$regex":
				hasRegex = true
				switch r := op.Value.(type) {
				case bson.Regex:
					regexPattern = r.Pattern
					if regexOptions == "" {
						regexOptions = r.Options
					}
				case string:
					regexPattern = r
				default:
					return &cmdError{
						code:     51091,
						codeName: "BadValue",
						msg:      "$regex has to be a string",
					}
				}
			case "$options":
				if s, ok := op.Value.(string); ok {
					regexOptions = s
				}
			case "$not":
				if err := validateFieldCond(op.Value); err != nil {
					return err
				}
			case "$elemMatch":
				if sub, ok := asDoc(op.Value); ok {
					if err := validateFilter(sub); err != nil {
						return err
					}
				}
			}
		}
		if hasRegex {
			if _, err := compileRegex(regexPattern, regexOptions); err != nil {
				return err
			}
		}
		return nil
	}
	if rx, ok := cond.(bson.Regex); ok {
		if _, err := compileRegex(rx.Pattern, rx.Options); err != nil {
			return err
		}
	}
	return nil
}

func matchDoc(doc bson.D, filter bson.D) (bool, error) {
	for _, elem := range filter {
		switch elem.Key {
		case "$and":
			arr, ok := asArray(elem.Value)
			if !ok {
				return false, nil
			}
			for _, item := range arr {
				sub, ok := asDoc(item)
				if !ok {
					return false, nil
				}
				m, err := matchDoc(doc, sub)
				if err != nil {
					return false, err
				}
				if !m {
					return false, nil
				}
			}
		case "$or":
			arr, ok := asArray(elem.Value)
			if !ok {
				return false, nil
			}
			matched := false
			for _, item := range arr {
				sub, ok := asDoc(item)
				if !ok {
					continue
				}
				m, err := matchDoc(doc, sub)
				if err != nil {
					return false, err
				}
				if m {
					matched = true
					break
				}
			}
			if !matched {
				return false, nil
			}
		case "$nor":
			arr, ok := asArray(elem.Value)
			if !ok {
				return false, nil
			}
			for _, item := range arr {
				sub, ok := asDoc(item)
				if !ok {
					continue
				}
				m, err := matchDoc(doc, sub)
				if err != nil {
					return false, err
				}
				if m {
					return false, nil
				}
			}
		default:
			val, exists := docGetPath(doc, elem.Key)
			m, err := matchField(val, exists, elem.Value)
			if err != nil {
				return false, err
			}
			if !m {
				return false, nil
			}
		}
	}
	return true, nil
}

func matchField(val any, exists bool, cond any) (bool, error) {
	if opDoc, ok := asDoc(cond); ok && isOperatorDoc(opDoc) {
		var regexPattern string
		var regexOptions string
		hasRegex := false

		for _, op := range opDoc {
			switch op.Key {
			case "$eq":
				if !matchScalarOrArrayElem(val, exists, op.Value) {
					return false, nil
				}
			case "$ne":
				if matchScalarOrArrayElem(val, exists, op.Value) {
					return false, nil
				}
			case "$gt":
				if !exists || val == nil || !compareWithArray(val, op.Value, func(c int) bool { return c > 0 }) {
					return false, nil
				}
			case "$gte":
				if !exists || val == nil || !compareWithArray(val, op.Value, func(c int) bool { return c >= 0 }) {
					return false, nil
				}
			case "$lt":
				if !exists || val == nil || !compareWithArray(val, op.Value, func(c int) bool { return c < 0 }) {
					return false, nil
				}
			case "$lte":
				if !exists || val == nil || !compareWithArray(val, op.Value, func(c int) bool { return c <= 0 }) {
					return false, nil
				}
			case "$in":
				arr, ok := asArray(op.Value)
				if !ok {
					return false, nil
				}
				found := false
				for _, target := range arr {
					if matchScalarOrArrayElem(val, exists, target) {
						found = true
						break
					}
				}
				if !found {
					return false, nil
				}
			case "$nin":
				arr, ok := asArray(op.Value)
				if !ok {
					return false, nil
				}
				for _, target := range arr {
					if matchScalarOrArrayElem(val, exists, target) {
						return false, nil
					}
				}
			case "$exists":
				want, _ := asBool(op.Value)
				if exists != want {
					return false, nil
				}
			case "$not":
				m, err := matchField(val, exists, op.Value)
				if err != nil {
					return false, err
				}
				if m {
					return false, nil
				}
			case "$regex":
				hasRegex = true
				switch r := op.Value.(type) {
				case bson.Regex:
					regexPattern = r.Pattern
					if regexOptions == "" {
						regexOptions = r.Options
					}
				case string:
					regexPattern = r
				default:
					return false, &cmdError{
						code:     51091,
						codeName: "BadValue",
						msg:      "$regex has to be a string",
					}
				}
			case "$options":
				if s, ok := op.Value.(string); ok {
					regexOptions = s
				}
			case "$elemMatch":
				sub, ok := asDoc(op.Value)
				if !ok {
					return false, nil
				}
				arr, ok := asArray(val)
				if !ok {
					return false, nil
				}
				matched := false
				for _, item := range arr {
					if d, ok := asDoc(item); ok {
						m, err := matchDoc(d, sub)
						if err != nil {
							return false, err
						}
						if m {
							matched = true
							break
						}
					}
				}
				if !matched {
					return false, nil
				}
			default:
				return false, nil
			}
		}

		if hasRegex {
			m, err := matchRegex(val, exists, regexPattern, regexOptions)
			if err != nil {
				return false, err
			}
			if !m {
				return false, nil
			}
		}
		return true, nil
	}

	if rx, ok := cond.(bson.Regex); ok {
		return matchRegex(val, exists, rx.Pattern, rx.Options)
	}

	return matchScalarOrArrayElem(val, exists, cond), nil
}

func matchScalarOrArrayElem(val any, exists bool, target any) bool {
	if !exists {
		return target == nil
	}
	if valuesEqual(val, target) {
		return true
	}
	if arr, ok := asArray(val); ok {
		for _, item := range arr {
			if valuesEqual(item, target) {
				return true
			}
		}
	}
	return false
}

func compareWithArray(val, target any, pred func(int) bool) bool {
	if arr, ok := asArray(val); ok {
		for _, item := range arr {
			if sameComparableCategory(item, target) && pred(compareBSONValues(item, target)) {
				return true
			}
		}
		return false
	}
	if !sameComparableCategory(val, target) {
		return false
	}
	return pred(compareBSONValues(val, target))
}

func compileRegex(pattern, options string) (*regexp.Regexp, error) {
	flags := ""
	if strings.Contains(options, "i") {
		flags += "i"
	}
	if strings.Contains(options, "m") {
		flags += "m"
	}
	if strings.Contains(options, "s") {
		flags += "s"
	}
	if flags != "" {
		pattern = "(?" + flags + ")" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, &cmdError{
			code:     51091,
			codeName: "BadValue",
			msg:      fmt.Sprintf("Regular expression is invalid: %v", err),
		}
	}
	return re, nil
}

func matchRegex(val any, exists bool, pattern, options string) (bool, error) {
	re, err := compileRegex(pattern, options)
	if err != nil {
		return false, err
	}
	if !exists || val == nil {
		return false, nil
	}
	if s, ok := val.(string); ok {
		return re.MatchString(s), nil
	}
	if arr, ok := asArray(val); ok {
		for _, item := range arr {
			if s, ok := item.(string); ok && re.MatchString(s) {
				return true, nil
			}
		}
	}
	return false, nil
}

func isOperatorDoc(d bson.D) bool {
	for _, elem := range d {
		if strings.HasPrefix(elem.Key, "$") {
			return true
		}
	}
	return false
}

// --- Updates & Projections ---

func applyUpdate(doc bson.D, update bson.D, isInsert bool) (bson.D, error) {
	if !isOperatorDoc(update) {
		// Full replacement document
		res := cloneDoc(update)
		if idVal, hasID := docGet(doc, "_id"); hasID {
			if _, newHasID := docGet(res, "_id"); !newHasID {
				res = append(bson.D{{Key: "_id", Value: idVal}}, res...)
			}
		}
		return res, nil
	}

	res := cloneDoc(doc)
	for _, elem := range update {
		opDoc, ok := asDoc(elem.Value)
		if !ok {
			return nil, fmt.Errorf("update operator %s requires document", elem.Key)
		}
		switch elem.Key {
		case "$set":
			for _, f := range opDoc {
				res = docSetPath(res, f.Key, cloneVal(f.Value))
			}
		case "$setOnInsert":
			if isInsert {
				for _, f := range opDoc {
					res = docSetPath(res, f.Key, cloneVal(f.Value))
				}
			}
		case "$unset":
			for _, f := range opDoc {
				res = docDeletePath(res, f.Key)
			}
		case "$inc":
			for _, f := range opDoc {
				cur, exists := docGetPath(res, f.Key)
				res = docSetPath(res, f.Key, addNumbers(cur, exists, f.Value))
			}
		case "$push":
			for _, f := range opDoc {
				cur, exists := docGetPath(res, f.Key)
				var arr bson.A
				if exists {
					if existing, ok := asArray(cur); ok {
						arr = existing
					}
				}
				arr = append(arr, cloneVal(f.Value))
				res = docSetPath(res, f.Key, arr)
			}
		case "$addToSet":
			for _, f := range opDoc {
				cur, exists := docGetPath(res, f.Key)
				var arr bson.A
				if exists {
					if existing, ok := asArray(cur); ok {
						arr = existing
					}
				}
				dup := false
				for _, item := range arr {
					if valuesEqual(item, f.Value) {
						dup = true
						break
					}
				}
				if !dup {
					arr = append(arr, cloneVal(f.Value))
				}
				res = docSetPath(res, f.Key, arr)
			}
		default:
			return nil, fmt.Errorf("unsupported update operator: %s", elem.Key)
		}
	}
	return res, nil
}

func buildUpsertBase(filter bson.D) bson.D {
	var base bson.D
	for _, elem := range filter {
		if strings.HasPrefix(elem.Key, "$") {
			continue
		}
		if d, ok := asDoc(elem.Value); ok && isOperatorDoc(d) {
			if eqVal, hasEq := docGet(d, "$eq"); hasEq {
				base = docSetPath(base, elem.Key, cloneVal(eqVal))
			}
			continue
		}
		base = docSetPath(base, elem.Key, cloneVal(elem.Value))
	}
	return base
}

func addNumbers(cur any, exists bool, delta any) any {
	if !exists || cur == nil {
		return cloneVal(delta)
	}
	if ci, ok := asInt64Strict(cur); ok {
		if di, ok := asInt64Strict(delta); ok {
			res := ci + di
			if res >= math.MinInt32 && res <= math.MaxInt32 {
				if _, is32 := cur.(int32); is32 {
					if _, d32 := delta.(int32); d32 {
						return int32(res)
					}
				}
			}
			return res
		}
	}
	cf, ok1 := asNumericFloat(cur)
	df, ok2 := asNumericFloat(delta)
	if ok1 && ok2 {
		return cf + df
	}
	return cur
}

func applyProjection(doc bson.D, proj bson.D) bson.D {
	if len(proj) == 0 {
		return doc
	}
	includeMode := false
	excludeID := false
	for _, elem := range proj {
		if elem.Key == "_id" {
			if n, ok := asInt64(elem.Value); ok && n == 0 {
				excludeID = true
			}
			continue
		}
		if n, ok := asInt64(elem.Value); ok && n != 0 {
			includeMode = true
		} else if b, ok := elem.Value.(bool); ok && b {
			includeMode = true
		} else if s, ok := elem.Value.(string); ok && strings.HasPrefix(s, "$") {
			includeMode = true
		}
	}

	if includeMode {
		out := bson.D{}
		if !excludeID {
			if idVal, ok := docGet(doc, "_id"); ok {
				out = append(out, bson.E{Key: "_id", Value: cloneVal(idVal)})
			}
		}
		for _, elem := range proj {
			if elem.Key == "_id" {
				continue
			}
			if s, ok := elem.Value.(string); ok && strings.HasPrefix(s, "$") {
				if v, exists := docGetPath(doc, s[1:]); exists {
					out = docSetPath(out, elem.Key, cloneVal(v))
				}
				continue
			}
			if n, ok := asInt64(elem.Value); ok && n != 0 {
				if v, exists := docGetPath(doc, elem.Key); exists {
					out = docSetPath(out, elem.Key, cloneVal(v))
				}
			} else if b, ok := elem.Value.(bool); ok && b {
				if v, exists := docGetPath(doc, elem.Key); exists {
					out = docSetPath(out, elem.Key, cloneVal(v))
				}
			}
		}
		return out
	}

	out := cloneDoc(doc)
	for _, elem := range proj {
		if n, ok := asInt64(elem.Value); ok && n == 0 {
			out = docDeletePath(out, elem.Key)
		} else if b, ok := elem.Value.(bool); ok && !b {
			out = docDeletePath(out, elem.Key)
		}
	}
	return out
}

// --- Unique Index Checking ---

func checkUniqueConflict(coll *collection, candidate bson.D, skipDocIdx int) (string, bool) {
	for _, idx := range coll.indexes {
		if !idx.Unique {
			continue
		}
		for i, existing := range coll.docs {
			if i == skipDocIdx {
				continue
			}
			if indexKeysCollide(candidate, existing, idx.Keys) {
				return idx.Name, true
			}
		}
	}
	return "", false
}

func indexKeysCollide(a, b, keys bson.D) bool {
	if len(keys) == 0 {
		return false
	}
	for _, k := range keys {
		va, ea := docGetPath(a, k.Key)
		vb, eb := docGetPath(b, k.Key)
		if !ea && !eb {
			continue
		}
		if ea != eb {
			return false
		}
		if !valuesEqual(va, vb) {
			return false
		}
	}
	return true
}

// --- Sorting & Pagination ---

func sortDocs(docs []bson.D, sortSpec bson.D) {
	if len(docs) <= 1 || len(sortSpec) == 0 {
		return
	}
	slices.SortStableFunc(docs, func(a, b bson.D) int {
		return compareDocsBySort(a, b, sortSpec)
	})
}

func compareDocsBySort(a, b, sortSpec bson.D) int {
	for _, elem := range sortSpec {
		dir, _ := asInt64(elem.Value)
		va, _ := docGetPath(a, elem.Key)
		vb, _ := docGetPath(b, elem.Key)
		cmp := compareBSONValues(va, vb)
		if cmp != 0 {
			if dir < 0 {
				return -cmp
			}
			return cmp
		}
	}
	return 0
}

func applySkipLimit(docs []bson.D, skip, limit int64) []bson.D {
	if skip > 0 {
		if int(skip) >= len(docs) {
			return nil
		}
		docs = docs[skip:]
	}
	if limit < 0 {
		limit = -limit
	}
	if limit > 0 && int(limit) < len(docs) {
		docs = docs[:limit]
	}
	return docs
}

// --- BSON Value Comparison & Helpers ---

func sameComparableCategory(a, b any) bool {
	return bsonTypeRank(a) == bsonTypeRank(b)
}

func bsonTypeRank(v any) int {
	switch v.(type) {
	case nil, bson.Null:
		return 1
	case int, int32, int64, float32, float64:
		return 2
	case string:
		return 3
	case bson.D:
		return 4
	case bson.A:
		return 5
	case bson.Binary:
		return 6
	case bson.ObjectID:
		return 7
	case bool:
		return 8
	case bson.DateTime, time.Time:
		return 9
	default:
		return 10
	}
}

func compareBSONValues(a, b any) int {
	ra := bsonTypeRank(a)
	rb := bsonTypeRank(b)
	if ra != rb {
		if ra < rb {
			return -1
		}
		return 1
	}

	switch ra {
	case 1:
		return 0
	case 2:
		fa, _ := asNumericFloat(a)
		fb, _ := asNumericFloat(b)
		if fa < fb {
			return -1
		}
		if fa > fb {
			return 1
		}
		return 0
	case 3:
		sa := a.(string)
		sb := b.(string)
		return strings.Compare(sa, sb)
	case 7:
		oa := a.(bson.ObjectID)
		ob := b.(bson.ObjectID)
		return bytes.Compare(oa[:], ob[:])
	case 8:
		ba := a.(bool)
		bb := b.(bool)
		if ba == bb {
			return 0
		}
		if !ba && bb {
			return -1
		}
		return 1
	case 9:
		ta := toMillis(a)
		tb := toMillis(b)
		if ta < tb {
			return -1
		}
		if ta > tb {
			return 1
		}
		return 0
	case 4:
		da := a.(bson.D)
		db := b.(bson.D)
		n := len(da)
		if len(db) < n {
			n = len(db)
		}
		for i := 0; i < n; i++ {
			if da[i].Key != db[i].Key {
				return strings.Compare(da[i].Key, db[i].Key)
			}
			if c := compareBSONValues(da[i].Value, db[i].Value); c != 0 {
				return c
			}
		}
		if len(da) < len(db) {
			return -1
		}
		if len(da) > len(db) {
			return 1
		}
		return 0
	default:
		return strings.Compare(fmt.Sprint(a), fmt.Sprint(b))
	}
}

func valuesEqual(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if bsonTypeRank(a) != bsonTypeRank(b) {
		return false
	}
	if arrA, ok := asArray(a); ok {
		arrB, okB := asArray(b)
		if !okB || len(arrA) != len(arrB) {
			return false
		}
		for i := range arrA {
			if !valuesEqual(arrA[i], arrB[i]) {
				return false
			}
		}
		return true
	}
	return compareBSONValues(a, b) == 0
}

func docsEqual(a, b bson.D) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Key != b[i].Key || !valuesEqual(a[i].Value, b[i].Value) {
			return false
		}
	}
	return true
}

func toMillis(v any) int64 {
	switch t := v.(type) {
	case bson.DateTime:
		return int64(t)
	case time.Time:
		return t.UTC().UnixMilli()
	default:
		return 0
	}
}

func docGet(d bson.D, key string) (any, bool) {
	for _, elem := range d {
		if elem.Key == key {
			return elem.Value, true
		}
	}
	return nil, false
}

func docGetPath(d bson.D, path string) (any, bool) {
	if !strings.Contains(path, ".") {
		return docGet(d, path)
	}
	parts := strings.Split(path, ".")
	var cur any = d
	for _, p := range parts {
		sub, ok := asDoc(cur)
		if ok {
			v, found := docGet(sub, p)
			if !found {
				return nil, false
			}
			cur = v
			continue
		}
		if arr, ok := asArray(cur); ok {
			idx, err := strconv.Atoi(p)
			if err != nil || idx < 0 || idx >= len(arr) {
				return nil, false
			}
			cur = arr[idx]
			continue
		}
		return nil, false
	}
	return cur, true
}

func docSet(d bson.D, key string, val any) bson.D {
	for i := range d {
		if d[i].Key == key {
			d[i].Value = val
			return d
		}
	}
	return append(d, bson.E{Key: key, Value: val})
}

func docSetPath(d bson.D, path string, val any) bson.D {
	if !strings.Contains(path, ".") {
		return docSet(d, path, val)
	}
	parts := strings.Split(path, ".")
	head := parts[0]
	rest := strings.Join(parts[1:], ".")
	existing, ok := docGet(d, head)
	var child bson.D
	if ok {
		if sub, isDoc := asDoc(existing); isDoc {
			child = cloneDoc(sub)
		}
	}
	child = docSetPath(child, rest, val)
	return docSet(d, head, child)
}

func docDeletePath(d bson.D, path string) bson.D {
	if !strings.Contains(path, ".") {
		out := make(bson.D, 0, len(d))
		for _, elem := range d {
			if elem.Key != path {
				out = append(out, elem)
			}
		}
		return out
	}
	parts := strings.Split(path, ".")
	head := parts[0]
	rest := strings.Join(parts[1:], ".")
	existing, ok := docGet(d, head)
	if !ok {
		return d
	}
	sub, ok := asDoc(existing)
	if !ok {
		return d
	}
	return docSet(d, head, docDeletePath(cloneDoc(sub), rest))
}

func docGetString(d bson.D, key string) (string, bool) {
	v, ok := docGet(d, key)
	if !ok {
		return "", false
	}
	return asString(v)
}

func docGetBool(d bson.D, key string) (bool, bool) {
	v, ok := docGet(d, key)
	if !ok {
		return false, false
	}
	return asBool(v)
}

func docGetInt64(d bson.D, key string) (int64, bool) {
	v, ok := docGet(d, key)
	if !ok {
		return 0, false
	}
	return asInt64(v)
}

func docGetDoc(d bson.D, key string) (bson.D, bool) {
	v, ok := docGet(d, key)
	if !ok {
		return nil, false
	}
	return asDoc(v)
}

func docGetArray(d bson.D, key string) (bson.A, bool) {
	v, ok := docGet(d, key)
	if !ok {
		return nil, false
	}
	return asArray(v)
}

func asString(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
}

func asBool(v any) (bool, bool) {
	b, ok := v.(bool)
	return b, ok
}

func asInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int32:
		return int64(n), true
	case int64:
		return n, true
	case float64:
		return int64(n), true
	default:
		return 0, false
	}
}

func asInt64Strict(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int32:
		return int64(n), true
	case int64:
		return n, true
	default:
		return 0, false
	}
}

func asFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	default:
		return 0, false
	}
}

func asNumericFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	default:
		return 0, false
	}
}

func asDoc(v any) (bson.D, bool) {
	switch d := v.(type) {
	case bson.D:
		return d, true
	case bson.M:
		out := make(bson.D, 0, len(d))
		for k, val := range d {
			out = append(out, bson.E{Key: k, Value: val})
		}
		return out, true
	default:
		return nil, false
	}
}

func asArray(v any) (bson.A, bool) {
	switch a := v.(type) {
	case bson.A:
		return a, true
	case []any:
		return bson.A(a), true
	default:
		return nil, false
	}
}

func cloneDoc(d bson.D) bson.D {
	if d == nil {
		return nil
	}
	out := make(bson.D, len(d))
	for i, elem := range d {
		out[i] = bson.E{Key: elem.Key, Value: cloneVal(elem.Value)}
	}
	return out
}

func cloneVal(v any) any {
	switch x := v.(type) {
	case bson.D:
		return cloneDoc(x)
	case bson.A:
		out := make(bson.A, len(x))
		for i, item := range x {
			out[i] = cloneVal(item)
		}
		return out
	case []byte:
		cp := make([]byte, len(x))
		copy(cp, x)
		return cp
	case bson.Binary:
		cp := make([]byte, len(x.Data))
		copy(cp, x.Data)
		return bson.Binary{Subtype: x.Subtype, Data: cp}
	default:
		return v
	}
}
