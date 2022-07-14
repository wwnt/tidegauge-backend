package test

import (
	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/require"
	"net"
	"testing"
)

// Test if out-of-order reading block will block
func TestYamuxOutOfOrder(t *testing.T) {
	conn1, conn2 := net.Pipe() // conn1: client

	clientSession, err := yamux.Client(conn1, nil)
	require.NoError(t, err)

	serverSession, err := yamux.Server(conn2, nil)
	require.NoError(t, err)

	var bs1 = []byte{1, 2}
	var bs2 = []byte{3, 4, 5}
	go func() {
		clientStream1, err := clientSession.Open()
		require.NoError(t, err)
		n, err := clientStream1.Write(bs1)
		require.NoError(t, err)
		require.Equal(t, len(bs1), n)

		clientStream2, err := clientSession.Open()
		require.NoError(t, err)
		n, err = clientStream2.Write(bs2)
		require.NoError(t, err)
		require.Equal(t, len(bs2), n)
	}()

	serverStream1, err := serverSession.Accept()
	serverStream2, err := serverSession.Accept()
	var tmp = make([]byte, 100)
	n, err := serverStream2.Read(tmp)
	require.NoError(t, err)
	require.Equal(t, len(bs2), n)
	require.EqualValues(t, bs2, tmp)

	n, err = serverStream1.Read(tmp)
	require.NoError(t, err)
	require.Equal(t, len(bs1), n)
	require.EqualValues(t, bs1, tmp)
}
