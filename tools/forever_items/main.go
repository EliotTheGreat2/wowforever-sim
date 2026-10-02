// Command forever_items merges WoW Forever items from a Wowhead gear-planner dump into
// assets/database/db.json and rebuilds db.bin.
//
// Blizzard keeps Forever's new items encrypted until they drop on live servers, so this
// runs after launch (Nov 4, 2026) or whenever Wowhead's Forever database fills in:
//
//	go run ./tools/forever_items -fetch -branch forever     # download + merge
//	go run ./tools/forever_items                            # merge files already downloaded
//
// -fetch downloads Wowhead's gear-planner data and one tooltip per item (stats come from
// tooltips, like tools/database/gen_db). The Wowhead branch name for Forever isn't known
// before launch: check the URL of any Forever item page on wowhead.com
// (wowhead.com/<branch>/item=...) and pass that as -branch.
//
// Items already in the database are replaced when Forever changed them (Forever re-statted
// some vanilla items); new items are added. Items with no stats and no effects are skipped.
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/wowsims/sod/sim/core/proto"
	"github.com/wowsims/sod/tools"
	"github.com/wowsims/sod/tools/database"
	"google.golang.org/protobuf/encoding/protojson"
	googleProto "google.golang.org/protobuf/proto"
)

func main() {
	dump := flag.String("dump", "assets/db_inputs/forever/wowhead_gearplanner_forever.txt", "Wowhead gear-planner page data for WoW Forever")
	dbPath := flag.String("db", "assets/database/db.json", "database to merge into")
	tooltips := flag.String("tooltips", "assets/db_inputs/forever/wowhead_item_tooltips_forever.csv", "Wowhead item tooltips for WoW Forever")
	fetch := flag.Bool("fetch", false, "download the gear-planner dump and item tooltips from Wowhead first")
	branch := flag.String("branch", "forever", "Wowhead site branch for WoW Forever (the path segment in wowhead.com/<branch>/item=...)")
	dryRun := flag.Bool("dry-run", false, "report changes without writing")
	flag.Parse()

	tm := &database.WowheadTooltipManager{TooltipManager: database.TooltipManager{
		FilePath:   *tooltips,
		UrlPattern: fmt.Sprintf("https://nether.wowhead.com/%s/tooltip/item/%%s?lvl=60", *branch),
	}}
	if *fetch {
		url := fmt.Sprintf("https://nether.wowhead.com/%s/data/gear-planner?dv=100", *branch)
		fmt.Println("downloading", url)
		tools.WriteFile(*dump, tools.ReadWebRequired(url))
	}

	raw, err := os.ReadFile(*dump)
	if err != nil {
		fail("reading dump: %v", err)
	}
	wh := database.ParseWowheadDB(string(raw))
	if len(wh.Items) == 0 {
		fail("no items found in %s: Wowhead's page format may have changed", *dump)
	}

	if *fetch {
		ids := make([]string, 0, len(wh.Items))
		for id := range wh.Items {
			ids = append(ids, id)
		}
		fmt.Printf("downloading %d item tooltips (cached ones are skipped)\n", len(ids))
		tm.Fetch(1, 0, ids)
	}
	tips := tm.Read()
	if len(tips) == 0 {
		fail("no item tooltips in %s: run with -fetch", *tooltips)
	}

	in, err := os.ReadFile(*dbPath)
	if err != nil {
		fail("reading db: %v", err)
	}
	db := &proto.UIDatabase{}
	if err := protojson.Unmarshal(in, db); err != nil {
		fail("parsing db: %v", err)
	}
	index := map[int32]int{}
	for i, it := range db.Items {
		index[it.Id] = i
	}

	added, changed, skipped := 0, 0, 0
	for _, whItem := range wh.Items {
		tip, ok := tips[whItem.ID]
		if !ok || !tip.IsEquippable() {
			skipped++
			continue
		}
		// Stats and effects from the tooltip, sources/phase/restrictions from the planner.
		item := tip.ToItemProto()
		googleProto.Merge(item, whItem.ToProto())
		if item.Id == 0 {
			skipped++
			continue
		}
		if i, ok := index[item.Id]; ok {
			if !googleProto.Equal(db.Items[i], item) {
				db.Items[i] = item
				changed++
			}
			continue
		}
		index[item.Id] = len(db.Items)
		db.Items = append(db.Items, item)
		added++
	}
	sort.Slice(db.Items, func(i, j int) bool { return db.Items[i].Id < db.Items[j].Id })
	fmt.Printf("Wowhead items: %d  added: %d  changed: %d  skipped: %d  total now: %d\n", len(wh.Items), added, changed, skipped, len(db.Items))
	if *dryRun {
		return
	}

	js, err := protojson.MarshalOptions{Indent: " "}.Marshal(db)
	if err != nil {
		fail("encoding db.json: %v", err)
	}
	if err := os.WriteFile(*dbPath, js, 0644); err != nil {
		fail("writing db.json: %v", err)
	}
	bin, err := googleProto.Marshal(db)
	if err != nil {
		fail("encoding db.bin: %v", err)
	}
	if err := os.WriteFile("assets/database/db.bin", bin, 0644); err != nil {
		fail("writing db.bin: %v", err)
	}
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "forever_items: "+format+"\n", args...)
	os.Exit(1)
}
