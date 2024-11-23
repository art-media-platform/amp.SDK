


namespace Amp.Std {

    public static partial class Spec {
        public static readonly TagID   MetaNodeID       = new(0, 0, 2701);

        public static readonly TagExpr TagRoot          = new TagExpr().With("amp");
        public static readonly TagExpr AttrSpec         = TagRoot.With("attr");
        public static readonly TagExpr AppSpec          = TagRoot.With("app");
        public static readonly TagExpr CellChildren     = AttrSpec.With("children.Tag.ID");
        public static readonly TagExpr CellProperties   = AttrSpec.With("cell-properties");

        public static readonly TagExpr Property         = new TagExpr().With("cell-property");

        public static readonly TagExpr Glyphs           = Property.With("Tags.glyphs");
        public static readonly TagExpr Links            = Property.With("Tags.links");

	    public static readonly TagID   CellMediaID      = Property.With("Tag.content.media").ID;
	    public static readonly TagID   CellCover        = Property.With("Tag.content.cover").ID;
	    public static readonly TagID   CellVis          = Property.With("Tag.content.vis").ID;

        public static readonly TagExpr TextTag          = Property.With("Tag.text");
	    public static readonly TagID   CellLabel        = TextTag.With("label").ID;
	    public static readonly TagID   CellCaption      = TextTag.With("caption").ID;
	    public static readonly TagID   CellCollection   = TextTag.With("collection").ID;

        // Meta attrs registered (allowed) to be pushed
        public static readonly TagExpr Login           = Registry.RegisterPrototype<Login>();
        public static readonly TagExpr LoginChallenge  = Registry.RegisterPrototype<LoginChallenge>();
        public static readonly TagExpr LoginResponse   = Registry.RegisterPrototype<LoginResponse>();
        public static readonly TagExpr LoginCheckpoint = Registry.RegisterPrototype<LoginCheckpoint>();
        public static readonly TagExpr PinRequest      = Registry.RegisterPrototype<PinRequest>();
        public static readonly TagExpr LaunchURL       = Registry.RegisterPrototype<LaunchURL>();
        public static readonly TagID   Err             = Registry.RegisterPrototype<Err>().ID;
        public static readonly TagID   Tag             = Registry.RegisterPrototype<Tag>().ID;

    }

}
