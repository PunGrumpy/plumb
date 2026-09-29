package sandesh

import (
	"strings"
	"testing"
)

const routeResp = `<?xml-stylesheet type="text/xsl" href="/universal_parse.xsl"?>
<ShowRouteResp type="sandesh">
  <tables type="list" identifier="1">
    <list type="struct" size="1">
      <ShowRouteTable>
        <routing_table_name type="string">a:b:vn1:vn1.inet.0</routing_table_name>
        <routes type="list">
          <list type="struct" size="1">
            <ShowRoute>
              <prefix type="string">10.0.1.5/32</prefix>
              <paths type="list">
                <list type="struct" size="1">
                  <ShowRoutePath>
                    <protocol type="string">XMPP</protocol>
                    <tunnel_encap type="list">
                      <list type="string" size="2">
                        <element>gre</element>
                        <element>udp</element>
                      </list>
                    </tunnel_encap>
                  </ShowRoutePath>
                </list>
              </paths>
            </ShowRoute>
          </list>
        </routes>
      </ShowRouteTable>
    </list>
  </tables>
</ShowRouteResp>`

func TestParse(t *testing.T) {
	doc, err := Parse(strings.NewReader(routeResp))
	if err != nil {
		t.Fatal(err)
	}
	tables := doc.FindAll("ShowRouteTable")
	if len(tables) != 1 {
		t.Fatalf("got %d tables", len(tables))
	}
	if got := tables[0].Str("routing_table_name"); got != "a:b:vn1:vn1.inet.0" {
		t.Errorf("table name = %q", got)
	}
	path := tables[0].Find("ShowRoutePath")
	if path.Str("protocol") != "XMPP" {
		t.Errorf("protocol = %q", path.Str("protocol"))
	}
	if got := strings.Join(path.Strings("tunnel_encap"), ","); got != "gre,udp" {
		t.Errorf("encap = %q", got)
	}
	if path.Child("tunnel_encap").Type != "list" {
		t.Error("type attribute not kept")
	}
}

func TestMissingFieldsAreZero(t *testing.T) {
	var n *Node
	if n.Str("x") != "" || n.Int("x") != -1 || n.Find("x") != nil || n.Strings("x") != nil {
		t.Error("nil node should read as empty")
	}
}
