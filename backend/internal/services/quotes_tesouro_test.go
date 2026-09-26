package services

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// failingReader yields a CSV prefix, then returns a non-EOF error forever —
// what an HTTP body does after its request context times out mid-download.
type failingReader struct {
	r   io.Reader
	err error
}

func (f *failingReader) Read(p []byte) (int, error) {
	n, err := f.r.Read(p)
	if err == io.EOF {
		return n, f.err
	}
	return n, err
}

func TestParseTesouroDiretoCSVReturnsOnReadError(t *testing.T) {
	csvPrefix := "Tipo Titulo;Data Vencimento;Data Base;Taxa Compra Manha;Taxa Venda Manha;PU Compra Manha;PU Venda Manha;PU Base Manha\n" +
		"Tesouro IPCA+;15/05/2035;01/09/2026;6,5;6,6;3000,00;2990,00;2990,00\n" +
		"Tesouro IPCA+;15/05/2035;02/09/2026;6,5;6,6;3001,00;29"
	body := &failingReader{r: strings.NewReader(csvPrefix), err: errors.New("context deadline exceeded")}

	done := make(chan error, 1)
	go func() {
		_, err := parseTesouroDiretoCSV(body)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error from truncated download, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("parseTesouroDiretoCSV spun forever on a persistent read error")
	}
}

func TestParseTesouroDiretoCSVSkipsMalformedRows(t *testing.T) {
	csvData := "Tipo Titulo;Data Vencimento;Data Base;Taxa Compra Manha;Taxa Venda Manha;PU Compra Manha;PU Venda Manha;PU Base Manha\n" +
		"bad \"quote;row\n" +
		"Tesouro Selic;01/03/2029;01/09/2026;0,1;0,2;15000,00;14990,00;14990,00\n"
	out, err := parseTesouroDiretoCSV(strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p, ok := out["Tesouro Selic (2029)"]; !ok || p.PUVenda != 14990 {
		t.Fatalf("expected Tesouro Selic (2029) at 14990, got %+v", out)
	}
}
