package proto

// Razer's cloud EQ library, as Synapse fetched it. Every entry carries the
// name Synapse shows, the ten band values, and the footstep scaling curve that
// rides alongside them.
//
// Lifted from the availableCloudEQ payload in the Synapse logs, not typed out
// by hand. Cross-checked against the headset: eight of the nine slots stored on
// this device match their library entry exactly, band for band.
//
// The headset records which library entry a slot came from as cloudEqId, so
// this is also how a slot gets a real name rather than a number.

// LibraryEntry is one preset from Razer's cloud library.
type LibraryEntry struct {
	ID       byte
	Name     string
	Bands    EQ
	Footstep EQ
}

// Library is every preset seen in the capture, ordered by id.
var Library = []LibraryEntry{
	{ID: 0, Name: "Default", Bands: EQ{0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, Footstep: EQ{0, 0, 1, 1, 0, 0, 0, 0, 0, 0}},
	{ID: 1, Name: "Game", Bands: EQ{2, 2, 5, 5, 1, -1, 2, 3, 3, 3}, Footstep: EQ{0, 0, 1, 1, 0, 0, 0, 0, 0, 0}},
	{ID: 2, Name: "Music", Bands: EQ{2, 2, 0, 0, 1, -1, -1, 3, 3, 3}, Footstep: EQ{0, 0, 1, 1, 0, 0, 0, 0, 0, 0}},
	{ID: 3, Name: "Movie", Bands: EQ{3, 3, 3, -1, -4, -4, 2, 3, 3, 3}, Footstep: EQ{0, 0, 1, 1, 0, 0, 0, 0, 0, 0}},
	{ID: 10, Name: "Valorant", Bands: EQ{1, 1, -1, 0, 2, 0, 4, 4, 4, -3}, Footstep: EQ{0, 0, 1, 1, 1, 0, 0, 0, 0, 0}},
	{ID: 11, Name: "Counter-Strike 2", Bands: EQ{-2, -1, -1, -2, 3, 3, 0, 1, 2, 1}, Footstep: EQ{0, 0, 0, 1, 1, 1, 0, 0, 0, 0}},
	{ID: 12, Name: "Fortnite", Bands: EQ{-4, -3, -1, 3, -2, -2, 4, 4, 4, 0}, Footstep: EQ{0, 1, 1, 1, 0, 0, 0, 0, 0, 0}},
	{ID: 13, Name: "Call of Duty", Bands: EQ{-2, 0, 3, 3, 3, 0, 0, 3, 3, 0}, Footstep: EQ{0, 0, 1, 1, 1, 0, 0, 0, 0, 0}},
	{ID: 14, Name: "Apex Legends", Bands: EQ{2, 2, 3, 3, -1, -2, 1, 4, 2, 2}, Footstep: EQ{0, 0, 1, 1, 1, 0, 0, 0, 0, 0}},
	{ID: 15, Name: "Halo Infinite", Bands: EQ{0, 0, 2, 2, 2, 1, 1, 2, 2, 2}, Footstep: EQ{0, 0, 1, 1, 1, 0, 0, 0, 0, 0}},
	{ID: 16, Name: "CS2 · NiKo", Bands: EQ{-2, -1, -1, -2, 3, 3, 0, 1, 2, 1}, Footstep: EQ{0, 0, 0, 1, 1, 1, 0, 0, 0, 0}},
	{ID: 17, Name: "Valorant · Mako · DRX", Bands: EQ{-1, 0, 1, 1, -1, -1, 0, 0, -1, -2}, Footstep: EQ{0, 0, 1, 1, 1, 0, 0, 0, 0, 0}},
	{ID: 18, Name: "Apex Legends · Hakis · Alliance", Bands: EQ{-1, 0, 0, 0, -1, 0, 2, 3, 2, 3}, Footstep: EQ{0, 0, 1, 1, 1, 0, 0, 0, 0, 0}},
	{ID: 19, Name: "Fortnite · PodaSai · Gentle Mates", Bands: EQ{0, 0, 1, 1, 0, 0, 0, 1, 0, 0}, Footstep: EQ{0, 1, 1, 1, 0, 0, 0, 0, 0, 0}},
	{ID: 20, Name: "CoD · Shotzzy · Optic", Bands: EQ{-6, -2, 1, 4, 3, -1, 0, -2, -4, -3}, Footstep: EQ{0, 0, 1, 1, 1, 0, 0, 0, 0, 0}},
	{ID: 21, Name: "Halo · Snip3down", Bands: EQ{-3, -1, 3, 4, 5, 2, 1, -1, -1, 0}, Footstep: EQ{0, 0, 1, 1, 1, 0, 0, 0, 0, 0}},
	{ID: 22, Name: "CS2 · Spinx · MOUZ", Bands: EQ{-2, -1, -1, -2, 3, 3, 0, 1, 2, 1}, Footstep: EQ{0, 0, 0, 1, 1, 1, 0, 0, 0, 0}},
	{ID: 23, Name: "Valorant · Zellsis · Sentinels", Bands: EQ{1, 4, -1, 4, 2, 3, 4, 4, 4, -3}, Footstep: EQ{0, 0, 1, 1, 1, 0, 0, 0, 0, 0}},
	{ID: 24, Name: "Apex Legends · Unlucky · Alliance", Bands: EQ{2, 2, 3, 0, -1, -2, 0, 2, 2, 2}, Footstep: EQ{0, 0, 1, 1, 1, 0, 0, 0, 0, 0}},
	{ID: 25, Name: "Fortnite · Vanyak3kk · Gentle Mates", Bands: EQ{1, 3, 3, 3, 2, 1, 1, 0, 2, 2}, Footstep: EQ{0, 1, 1, 1, 0, 0, 0, 0, 0, 0}},
	{ID: 26, Name: "CoD · Dashy · Optic", Bands: EQ{-6, -2, 1, 4, 3, -1, 0, -2, -4, -3}, Footstep: EQ{0, 0, 1, 1, 1, 0, 0, 0, 0, 0}},
	{ID: 27, Name: "Halo · LastShot · Shopify Rebellion", Bands: EQ{2, 1, 0, -2, 0, 2, 3, 4, 2, 0}, Footstep: EQ{0, 0, 1, 1, 1, 0, 0, 0, 0, 0}},
	{ID: 28, Name: "Delta Force", Bands: EQ{2, 2, -3, -3, -3, 4, 4, 4, 4, 4}, Footstep: EQ{0, 0, 1, 1, 1, 0, 0, 0, 0, 0}},
	{ID: 29, Name: "Battlefield 6", Bands: EQ{1, 1, 2, 2, 2, -1, -1, 4, 4, 3}, Footstep: EQ{0, 0, 1, 1, 1, 0, 0, 0, 0, 0}},
	{ID: 30, Name: "CS2 · dank1ng", Bands: EQ{-2, -1, -1, -2, 3, 4, 0, 1, 2, 1}, Footstep: EQ{0, 0, 0, 1, 1, 1, 0, 0, 0, 0}},
	{ID: 31, Name: "Valorant · sword9 · TYLOO", Bands: EQ{2, 2, 0, 0, 1, -1, -1, 3, 3, 3}, Footstep: EQ{0, 0, 1, 1, 1, 0, 0, 0, 0, 0}},
	{ID: 32, Name: "Valorant · slowly · TYLOO", Bands: EQ{1, 1, -1, 0, 2, 0, 4, 4, 4, -5}, Footstep: EQ{0, 0, 1, 1, 1, 0, 0, 0, 0, 0}},
	{ID: 33, Name: "Valorant · Splash · TYLOO", Bands: EQ{2, 2, 5, 5, 1, -1, 2, 3, 3, 3}, Footstep: EQ{0, 0, 1, 1, 1, 0, 0, 0, 0, 0}},
	{ID: 34, Name: "Valorant · Scales · TYLOO", Bands: EQ{1, 2, 1, 2, 3, 1, 4, 3, 1, 2}, Footstep: EQ{0, 0, 1, 1, 1, 0, 0, 0, 0, 0}},
	{ID: 35, Name: "Valorant · ERV · TYLOO", Bands: EQ{2, 2, 0, 0, 1, -1, -1, 3, 3, 3}, Footstep: EQ{0, 0, 1, 1, 1, 0, 0, 0, 0, 0}},
	{ID: 36, Name: "Valorant · cortezia · Sentinels", Bands: EQ{2, 2, 4, 4, 1, 0, 1, 3, 3, 3}, Footstep: EQ{0, 0, 1, 1, 1, 0, 0, 0, 0, 0}},
	{ID: 37, Name: "Valorant · johnqt · Sentinels", Bands: EQ{2, 2, 5, 5, 1, -1, 2, 3, 3, 3}, Footstep: EQ{0, 0, 1, 1, 1, 0, 0, 0, 0, 0}},
	{ID: 38, Name: "Valorant · Reduxx · Sentinels", Bands: EQ{1, 1, 1, 1, 1, 1, 1, 1, 1, 1}, Footstep: EQ{0, 0, 1, 1, 1, 0, 0, 0, 0, 0}},
	{ID: 39, Name: "Delta Force · BanJing · Jaguar Ace Gaming", Bands: EQ{1, -1, 1, 2, 2, 0, 3, 2, 3, 2}, Footstep: EQ{0, 0, 1, 1, 1, 0, 0, 0, 0, 0}},
	{ID: 40, Name: "Delta Force · GaoXin · Jaguar Ace Gaming", Bands: EQ{1, -1, 1, 2, 2, 0, 3, 2, 3, 2}, Footstep: EQ{0, 0, 1, 1, 1, 0, 0, 0, 0, 0}},
	{ID: 41, Name: "Delta Force · YERAN · Jaguar Ace Gaming", Bands: EQ{1, -1, 1, 2, 2, 0, 3, 2, 3, 2}, Footstep: EQ{0, 0, 1, 1, 1, 0, 0, 0, 0, 0}},
	{ID: 42, Name: "Delta Force · ZGOD · Jaguar Ace Gaming", Bands: EQ{1, -1, 1, 2, 2, 0, 3, 2, 3, 2}, Footstep: EQ{0, 0, 1, 1, 1, 0, 0, 0, 0, 0}},
	{ID: 43, Name: "Delta Force · Blank · TE 溯", Bands: EQ{-2, -2, 0, 1, 2, 0, 0, 1, -1, -1}, Footstep: EQ{0, 0, 1, 1, 1, 0, 0, 0, 0, 0}},
	{ID: 44, Name: "Delta Force · GuiRoW · TE 溯", Bands: EQ{-2, -2, 0, 1, 2, 0, 0, 1, -1, -1}, Footstep: EQ{0, 0, 1, 1, 1, 0, 0, 0, 0, 0}},
	{ID: 45, Name: "Delta Force · Kellyt · TE 溯", Bands: EQ{-2, -2, 0, 1, 2, 0, 0, 1, -1, -1}, Footstep: EQ{0, 0, 1, 1, 1, 0, 0, 0, 0, 0}},
	{ID: 46, Name: "Delta Force · Moxi · TE 溯", Bands: EQ{-2, -2, 0, 1, 2, 0, 0, 1, -1, -1}, Footstep: EQ{0, 0, 1, 1, 1, 0, 0, 0, 0, 0}},
}

// libraryByID indexes Library for lookup by the headset's cloudEqId.
var libraryByID = func() map[byte]LibraryEntry {
	m := make(map[byte]LibraryEntry, len(Library))
	for _, e := range Library {
		m[e.ID] = e
	}
	return m
}()

// LibraryName returns Synapse's name for a cloud EQ id, and whether it is known.
func LibraryName(id byte) (string, bool) {
	e, ok := libraryByID[id]
	return e.Name, ok
}

// LibraryEntryByID looks up a full preset.
func LibraryEntryByID(id byte) (LibraryEntry, bool) {
	e, ok := libraryByID[id]
	return e, ok
}

// LibraryByName finds a preset by its Synapse name, case sensitively.
func LibraryByName(name string) (LibraryEntry, bool) {
	for _, e := range Library {
		if e.Name == name {
			return e, true
		}
	}
	return LibraryEntry{}, false
}
