


namespace Amp.Std {

    public static partial class Spec {
        public static readonly TagID   MetaNodeID       = new(0, 0, 2701);

        public static readonly TagSpec TagRoot          = new TagSpec().With("amp");
        public static readonly TagSpec AttrSpec         = TagRoot.With("attr");
        public static readonly TagSpec AppSpec          = TagRoot.With("app");
        public static readonly TagSpec CellChildren     = AttrSpec.With("children.TagID");
        public static readonly TagSpec CellProperties   = AttrSpec.With("cell-properties");

        public static readonly TagSpec Property         = new TagSpec().With("cell-property");

        public static readonly TagSpec Glyphs           = Property.With("Tags.glyphs");
        public static readonly TagSpec Links            = Property.With("Tags.links");

	    public static readonly TagID   CellMediaID      = Property.With("Tag.content.media").ID;
	    public static readonly TagID   CellCover        = Property.With("Tag.content.cover").ID;
	    public static readonly TagID   CellVis          = Property.With("Tag.content.vis").ID;

        public static readonly TagSpec TextTag          = Property.With("Tag.text");
	    public static readonly TagID   CellLabel        = TextTag.With("label").ID;
	    public static readonly TagID   CellCaption      = TextTag.With("caption").ID;
	    public static readonly TagID   CellCollection   = TextTag.With("collection").ID;

        // Meta attrs registered (allowed) to be pushed
        public static readonly TagSpec Login           = Registry.RegisterPrototype<Login>();
        public static readonly TagSpec LoginChallenge  = Registry.RegisterPrototype<LoginChallenge>();
        public static readonly TagSpec LoginResponse   = Registry.RegisterPrototype<LoginResponse>();
        public static readonly TagSpec LoginCheckpoint = Registry.RegisterPrototype<LoginCheckpoint>();
        public static readonly TagSpec PinRequest      = Registry.RegisterPrototype<PinRequest>();
        public static readonly TagSpec LaunchURL       = Registry.RegisterPrototype<LaunchURL>();
        public static readonly TagID   Err             = Registry.RegisterPrototype<Err>().ID;
        public static readonly TagID   Tag             = Registry.RegisterPrototype<Tag>().ID;

    }

}
