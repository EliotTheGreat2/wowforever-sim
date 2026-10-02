// Command forever_db rebuilds db.bin from db.json after merging vanilla items
// from wowsims/classic into the database for WoW Forever.
package main

import (
	"os"

	"github.com/wowsims/sod/sim/core/proto"
	"google.golang.org/protobuf/encoding/protojson"
	googleProto "google.golang.org/protobuf/proto"
)

func main() {
	in, err := os.ReadFile("assets/database/db.json")
	if err != nil {
		panic(err)
	}
	db := &proto.UIDatabase{}
	if err := protojson.Unmarshal(in, db); err != nil {
		panic(err)
	}
	out, err := googleProto.Marshal(db)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile("assets/database/db.bin", out, 0644); err != nil {
		panic(err)
	}
	println("items:", len(db.Items), "bytes:", len(out))
}
