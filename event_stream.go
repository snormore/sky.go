package sky

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
)

//------------------------------------------------------------------------------
//
// Typedefs
//
//------------------------------------------------------------------------------

// An event stream maintains an open connection to the database to send events
// in bulk.
type EventStream struct {
	client  Client
	header  []byte
	encoder *json.Encoder
	buffer  *bufio.Writer
	conn    net.Conn
}

//------------------------------------------------------------------------------
//
// Constructor
//
//------------------------------------------------------------------------------

func NewEventStream(c Client, table Table) (*EventStream, error) {
	header := fmt.Sprintf("PATCH /tables/%s/events HTTP/1.0\r\nHost: %s\r\nContent-Type: application/json\r\nTransfer-Encoding: chunked\r\n\r\n", table.Name(), c.GetHost())
	return newStream(c, []byte(header))
}

func NewTableEventStream(c Client) (*EventStream, error) {
	header := fmt.Sprintf("PATCH /events HTTP/1.0\r\nHost: %s\r\nContent-Type: application/json\r\nTransfer-Encoding: chunked\r\n\r\n", c.GetHost())
	return newStream(c, []byte(header))
}

func newStream(c Client, header []byte) (*EventStream, error) {
	s := &EventStream{client: c, header: header}
	return s, s.Reconnect()
}

//------------------------------------------------------------------------------
//
// Methods
//
//------------------------------------------------------------------------------

//--------------------------------------
// Event API
//--------------------------------------

// Adds an event to an object.
func (s *EventStream) AddEvent(objectId string, event *Event) error {
	if objectId == "" {
		return errors.New("Object identifier required")
	}
	if event == nil {
		return errors.New("Event required")
	}

	// Attach the object identifier at the root of the event.
	data := event.Serialize()
	data["id"] = objectId

	// Encode the serialized data into the stream.
	return s.sendChunk(data)
}

func (s *EventStream) AddTableEvent(objectId string, table Table, event *Event) error {
	if objectId == "" {
		return errors.New("Object identifier required")
	}
	if table == nil {
		return errors.New("Table required")
	}
	if event == nil {
		return errors.New("Event required")
	}

	// Attach the object identifier at the root of the event.
	data := event.Serialize()
	data["id"] = objectId
	data["table"] = table.Name()
	return s.sendChunk(data)
}

func (s *EventStream) sendChunk(data map[string]interface{}) error {
	var err error
	var size int
	if data != nil {
		// Encode the serialized data into the stream.
		if err = s.encoder.Encode(data); err != nil {
			return err
		}
		size = s.buffer.Buffered()
	}

	if _, err = fmt.Fprintf(s.conn, "%x\r\n", size); err != nil {
		return err
	}
	if data != nil {
		if err = s.buffer.Flush(); err != nil {
			return err
		}
	} else {
	}
	if _, err = fmt.Fprint(s.conn, "\r\n"); err != nil {
		return err
	}
	return nil
}

func (s *EventStream) Close() error {
	defer s.conn.Close()
	if err := s.sendChunk(nil); err != nil {
		return err
	}
	response, err := http.ReadResponse(bufio.NewReader(s.conn), nil)
	if err != nil {
		return err
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		return errors.New(response.Status)
	}
	return nil
}

func (s *EventStream) Reconnect() error {
	if s.conn != nil {
		s.conn.Close()
	}
	address := fmt.Sprintf("%s:%d", s.client.GetHost(), s.client.GetPort())
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return err
	}
	if _, err = conn.Write(s.header); err != nil {
		conn.Close()
		return err
	}
	s.conn = conn
	s.buffer = bufio.NewWriter(conn)
	s.encoder = json.NewEncoder(s.buffer)
	return nil
}
