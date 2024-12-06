// <auto-generated>
//     Generated by the protocol buffer compiler.  DO NOT EDIT!
//     source: amp.av.proto
// </auto-generated>
#pragma warning disable 1591, 0612, 3021
#region Designer generated code

using pb = global::Google.Protobuf;
using pbc = global::Google.Protobuf.Collections;
using pbr = global::Google.Protobuf.Reflection;
using scg = global::System.Collections.Generic;
namespace Amp {

  /// <summary>Holder for reflection information generated from amp.av.proto</summary>
  public static partial class AmpAvReflection {

    #region Descriptor
    /// <summary>File descriptor for amp.av.proto</summary>
    public static pbr::FileDescriptor Descriptor {
      get { return descriptor; }
    }
    private static pbr::FileDescriptor descriptor;

    static AmpAvReflection() {
      byte[] descriptorData = global::System.Convert.FromBase64String(
          string.Concat(
            "CgxhbXAuYXYucHJvdG8SAmF2GhJhbXAvYW1wLmNvcmUucHJvdG8ikAEKCU1l",
            "ZGlhSXRlbRIdCgVGbGFncxgBIAEoDjIOLmF2Lk1lZGlhRmxhZ3MSHAoKU25h",
            "cHNob3RJRBgDIAEoCzIILmFtcC5UYWcSDwoHU3RhcnRBdBgFIAEoARIPCgdT",
            "ZWNvbmRzGAYgASgBEhIKClBvcHVsYXJpdHkYECABKAISEAoIT3JkZXJpbmcY",
            "ESABKAIqrwIKCk1lZGlhRmxhZ3MSEwoPTWVkaWFGbGFnc19Ob25lEAASFQoR",
            "TWVkaWFGbGFnc19Jc0xpdmUQARIZChVNZWRpYUZsYWdzX0lzU2Vla2FibGUQ",
            "AhIZChVNZWRpYUZsYWdzX0lzVW5lbmRpbmcQBBIZChVNZWRpYUZsYWdzX05l",
            "d0NoYXB0ZXIQCBIXChNNZWRpYUZsYWdzX05ld1RyYWNrEBASGAoTTWVkaWFG",
            "bGFnc19IYXNBdWRpbxCAAhIYChNNZWRpYUZsYWdzX0hhc1ZpZGVvEIAEEhkK",
            "FE1lZGlhRmxhZ3NfSGFzU3BlZWNoEIAIEhwKF01lZGlhRmxhZ3NfTmVlZHNO",
            "ZXR3b3JrEIAQEh4KF01lZGlhRmxhZ3NfU2tpcElzTGlrZWx5EICAgAhCBqoC",
            "A0FtcGIGcHJvdG8z"));
      descriptor = pbr::FileDescriptor.FromGeneratedCode(descriptorData,
          new pbr::FileDescriptor[] { global::Amp.AmpCoreReflection.Descriptor, },
          new pbr::GeneratedClrTypeInfo(new[] {typeof(global::Amp.MediaFlags), }, null, new pbr::GeneratedClrTypeInfo[] {
            new pbr::GeneratedClrTypeInfo(typeof(global::Amp.MediaItem), global::Amp.MediaItem.Parser, new[]{ "Flags", "SnapshotID", "StartAt", "Seconds", "Popularity", "Ordering" }, null, null, null, null)
          }));
    }
    #endregion

  }
  #region Enums
  public enum MediaFlags {
    [pbr::OriginalName("MediaFlags_None")] None = 0,
    [pbr::OriginalName("MediaFlags_IsLive")] IsLive = 1,
    [pbr::OriginalName("MediaFlags_IsSeekable")] IsSeekable = 2,
    [pbr::OriginalName("MediaFlags_IsUnending")] IsUnending = 4,
    [pbr::OriginalName("MediaFlags_NewChapter")] NewChapter = 8,
    [pbr::OriginalName("MediaFlags_NewTrack")] NewTrack = 16,
    [pbr::OriginalName("MediaFlags_HasAudio")] HasAudio = 256,
    [pbr::OriginalName("MediaFlags_HasVideo")] HasVideo = 512,
    [pbr::OriginalName("MediaFlags_HasSpeech")] HasSpeech = 1024,
    [pbr::OriginalName("MediaFlags_NeedsNetwork")] NeedsNetwork = 2048,
    /// <summary>
    /// When set, the user is more likely to skipping short intervals than switch media items.
    /// </summary>
    [pbr::OriginalName("MediaFlags_SkipIsLikely")] SkipIsLikely = 16777216,
  }

  #endregion

  #region Messages
  /// <summary>
  /// MediaItem wraps a media track, feature, featurette, collection, album, or playlist.
  /// </summary>
  public sealed partial class MediaItem : pb::IMessage<MediaItem>
  #if !GOOGLE_PROTOBUF_REFSTRUCT_COMPATIBILITY_MODE
      , pb::IBufferMessage
  #endif
  {
    private static readonly pb::MessageParser<MediaItem> _parser = new pb::MessageParser<MediaItem>(() => new MediaItem());
    private pb::UnknownFieldSet _unknownFields;
    [global::System.Diagnostics.DebuggerNonUserCodeAttribute]
    [global::System.CodeDom.Compiler.GeneratedCode("protoc", null)]
    public static pb::MessageParser<MediaItem> Parser { get { return _parser; } }

    [global::System.Diagnostics.DebuggerNonUserCodeAttribute]
    [global::System.CodeDom.Compiler.GeneratedCode("protoc", null)]
    public static pbr::MessageDescriptor Descriptor {
      get { return global::Amp.AmpAvReflection.Descriptor.MessageTypes[0]; }
    }

    [global::System.Diagnostics.DebuggerNonUserCodeAttribute]
    [global::System.CodeDom.Compiler.GeneratedCode("protoc", null)]
    pbr::MessageDescriptor pb::IMessage.Descriptor {
      get { return Descriptor; }
    }

    [global::System.Diagnostics.DebuggerNonUserCodeAttribute]
    [global::System.CodeDom.Compiler.GeneratedCode("protoc", null)]
    public MediaItem() {
      OnConstruction();
    }

    partial void OnConstruction();

    [global::System.Diagnostics.DebuggerNonUserCodeAttribute]
    [global::System.CodeDom.Compiler.GeneratedCode("protoc", null)]
    public MediaItem(MediaItem other) : this() {
      flags_ = other.flags_;
      snapshotID_ = other.snapshotID_ != null ? other.snapshotID_.Clone() : null;
      startAt_ = other.startAt_;
      seconds_ = other.seconds_;
      popularity_ = other.popularity_;
      ordering_ = other.ordering_;
      _unknownFields = pb::UnknownFieldSet.Clone(other._unknownFields);
    }

    [global::System.Diagnostics.DebuggerNonUserCodeAttribute]
    [global::System.CodeDom.Compiler.GeneratedCode("protoc", null)]
    public MediaItem Clone() {
      return new MediaItem(this);
    }

    /// <summary>Field number for the "Flags" field.</summary>
    public const int FlagsFieldNumber = 1;
    private global::Amp.MediaFlags flags_ = global::Amp.MediaFlags.None;
    [global::System.Diagnostics.DebuggerNonUserCodeAttribute]
    [global::System.CodeDom.Compiler.GeneratedCode("protoc", null)]
    public global::Amp.MediaFlags Flags {
      get { return flags_; }
      set {
        flags_ = value;
      }
    }

    /// <summary>Field number for the "SnapshotID" field.</summary>
    public const int SnapshotIDFieldNumber = 3;
    private global::Amp.Tag snapshotID_;
    /// <summary>
    /// hash of most recent snapshot (EditID?), and edit time UTC
    /// </summary>
    [global::System.Diagnostics.DebuggerNonUserCodeAttribute]
    [global::System.CodeDom.Compiler.GeneratedCode("protoc", null)]
    public global::Amp.Tag SnapshotID {
      get { return snapshotID_; }
      set {
        snapshotID_ = value;
      }
    }

    /// <summary>Field number for the "StartAt" field.</summary>
    public const int StartAtFieldNumber = 5;
    private double startAt_;
    /// <summary>
    /// starts playback at the given seconds offset
    /// </summary>
    [global::System.Diagnostics.DebuggerNonUserCodeAttribute]
    [global::System.CodeDom.Compiler.GeneratedCode("protoc", null)]
    public double StartAt {
      get { return startAt_; }
      set {
        startAt_ = value;
      }
    }

    /// <summary>Field number for the "Seconds" field.</summary>
    public const int SecondsFieldNumber = 6;
    private double seconds_;
    /// <summary>
    /// playback duration in seconds
    /// </summary>
    [global::System.Diagnostics.DebuggerNonUserCodeAttribute]
    [global::System.CodeDom.Compiler.GeneratedCode("protoc", null)]
    public double Seconds {
      get { return seconds_; }
      set {
        seconds_ = value;
      }
    }

    /// <summary>Field number for the "Popularity" field.</summary>
    public const int PopularityFieldNumber = 16;
    private float popularity_;
    /// <summary>
    /// 0.0 to 1.0
    /// </summary>
    [global::System.Diagnostics.DebuggerNonUserCodeAttribute]
    [global::System.CodeDom.Compiler.GeneratedCode("protoc", null)]
    public float Popularity {
      get { return popularity_; }
      set {
        popularity_ = value;
      }
    }

    /// <summary>Field number for the "Ordering" field.</summary>
    public const int OrderingFieldNumber = 17;
    private float ordering_;
    /// <summary>
    /// items are sorted by this value
    /// </summary>
    [global::System.Diagnostics.DebuggerNonUserCodeAttribute]
    [global::System.CodeDom.Compiler.GeneratedCode("protoc", null)]
    public float Ordering {
      get { return ordering_; }
      set {
        ordering_ = value;
      }
    }

    [global::System.Diagnostics.DebuggerNonUserCodeAttribute]
    [global::System.CodeDom.Compiler.GeneratedCode("protoc", null)]
    public override bool Equals(object other) {
      return Equals(other as MediaItem);
    }

    [global::System.Diagnostics.DebuggerNonUserCodeAttribute]
    [global::System.CodeDom.Compiler.GeneratedCode("protoc", null)]
    public bool Equals(MediaItem other) {
      if (ReferenceEquals(other, null)) {
        return false;
      }
      if (ReferenceEquals(other, this)) {
        return true;
      }
      if (Flags != other.Flags) return false;
      if (!object.Equals(SnapshotID, other.SnapshotID)) return false;
      if (!pbc::ProtobufEqualityComparers.BitwiseDoubleEqualityComparer.Equals(StartAt, other.StartAt)) return false;
      if (!pbc::ProtobufEqualityComparers.BitwiseDoubleEqualityComparer.Equals(Seconds, other.Seconds)) return false;
      if (!pbc::ProtobufEqualityComparers.BitwiseSingleEqualityComparer.Equals(Popularity, other.Popularity)) return false;
      if (!pbc::ProtobufEqualityComparers.BitwiseSingleEqualityComparer.Equals(Ordering, other.Ordering)) return false;
      return Equals(_unknownFields, other._unknownFields);
    }

    [global::System.Diagnostics.DebuggerNonUserCodeAttribute]
    [global::System.CodeDom.Compiler.GeneratedCode("protoc", null)]
    public override int GetHashCode() {
      int hash = 1;
      if (Flags != global::Amp.MediaFlags.None) hash ^= Flags.GetHashCode();
      if (snapshotID_ != null) hash ^= SnapshotID.GetHashCode();
      if (StartAt != 0D) hash ^= pbc::ProtobufEqualityComparers.BitwiseDoubleEqualityComparer.GetHashCode(StartAt);
      if (Seconds != 0D) hash ^= pbc::ProtobufEqualityComparers.BitwiseDoubleEqualityComparer.GetHashCode(Seconds);
      if (Popularity != 0F) hash ^= pbc::ProtobufEqualityComparers.BitwiseSingleEqualityComparer.GetHashCode(Popularity);
      if (Ordering != 0F) hash ^= pbc::ProtobufEqualityComparers.BitwiseSingleEqualityComparer.GetHashCode(Ordering);
      if (_unknownFields != null) {
        hash ^= _unknownFields.GetHashCode();
      }
      return hash;
    }

    [global::System.Diagnostics.DebuggerNonUserCodeAttribute]
    [global::System.CodeDom.Compiler.GeneratedCode("protoc", null)]
    public override string ToString() {
      return pb::JsonFormatter.ToDiagnosticString(this);
    }

    [global::System.Diagnostics.DebuggerNonUserCodeAttribute]
    [global::System.CodeDom.Compiler.GeneratedCode("protoc", null)]
    public void WriteTo(pb::CodedOutputStream output) {
    #if !GOOGLE_PROTOBUF_REFSTRUCT_COMPATIBILITY_MODE
      output.WriteRawMessage(this);
    #else
      if (Flags != global::Amp.MediaFlags.None) {
        output.WriteRawTag(8);
        output.WriteEnum((int) Flags);
      }
      if (snapshotID_ != null) {
        output.WriteRawTag(26);
        output.WriteMessage(SnapshotID);
      }
      if (StartAt != 0D) {
        output.WriteRawTag(41);
        output.WriteDouble(StartAt);
      }
      if (Seconds != 0D) {
        output.WriteRawTag(49);
        output.WriteDouble(Seconds);
      }
      if (Popularity != 0F) {
        output.WriteRawTag(133, 1);
        output.WriteFloat(Popularity);
      }
      if (Ordering != 0F) {
        output.WriteRawTag(141, 1);
        output.WriteFloat(Ordering);
      }
      if (_unknownFields != null) {
        _unknownFields.WriteTo(output);
      }
    #endif
    }

    #if !GOOGLE_PROTOBUF_REFSTRUCT_COMPATIBILITY_MODE
    [global::System.Diagnostics.DebuggerNonUserCodeAttribute]
    [global::System.CodeDom.Compiler.GeneratedCode("protoc", null)]
    void pb::IBufferMessage.InternalWriteTo(ref pb::WriteContext output) {
      if (Flags != global::Amp.MediaFlags.None) {
        output.WriteRawTag(8);
        output.WriteEnum((int) Flags);
      }
      if (snapshotID_ != null) {
        output.WriteRawTag(26);
        output.WriteMessage(SnapshotID);
      }
      if (StartAt != 0D) {
        output.WriteRawTag(41);
        output.WriteDouble(StartAt);
      }
      if (Seconds != 0D) {
        output.WriteRawTag(49);
        output.WriteDouble(Seconds);
      }
      if (Popularity != 0F) {
        output.WriteRawTag(133, 1);
        output.WriteFloat(Popularity);
      }
      if (Ordering != 0F) {
        output.WriteRawTag(141, 1);
        output.WriteFloat(Ordering);
      }
      if (_unknownFields != null) {
        _unknownFields.WriteTo(ref output);
      }
    }
    #endif

    [global::System.Diagnostics.DebuggerNonUserCodeAttribute]
    [global::System.CodeDom.Compiler.GeneratedCode("protoc", null)]
    public int CalculateSize() {
      int size = 0;
      if (Flags != global::Amp.MediaFlags.None) {
        size += 1 + pb::CodedOutputStream.ComputeEnumSize((int) Flags);
      }
      if (snapshotID_ != null) {
        size += 1 + pb::CodedOutputStream.ComputeMessageSize(SnapshotID);
      }
      if (StartAt != 0D) {
        size += 1 + 8;
      }
      if (Seconds != 0D) {
        size += 1 + 8;
      }
      if (Popularity != 0F) {
        size += 2 + 4;
      }
      if (Ordering != 0F) {
        size += 2 + 4;
      }
      if (_unknownFields != null) {
        size += _unknownFields.CalculateSize();
      }
      return size;
    }

    [global::System.Diagnostics.DebuggerNonUserCodeAttribute]
    [global::System.CodeDom.Compiler.GeneratedCode("protoc", null)]
    public void MergeFrom(MediaItem other) {
      if (other == null) {
        return;
      }
      if (other.Flags != global::Amp.MediaFlags.None) {
        Flags = other.Flags;
      }
      if (other.snapshotID_ != null) {
        if (snapshotID_ == null) {
          SnapshotID = new global::Amp.Tag();
        }
        SnapshotID.MergeFrom(other.SnapshotID);
      }
      if (other.StartAt != 0D) {
        StartAt = other.StartAt;
      }
      if (other.Seconds != 0D) {
        Seconds = other.Seconds;
      }
      if (other.Popularity != 0F) {
        Popularity = other.Popularity;
      }
      if (other.Ordering != 0F) {
        Ordering = other.Ordering;
      }
      _unknownFields = pb::UnknownFieldSet.MergeFrom(_unknownFields, other._unknownFields);
    }

    [global::System.Diagnostics.DebuggerNonUserCodeAttribute]
    [global::System.CodeDom.Compiler.GeneratedCode("protoc", null)]
    public void MergeFrom(pb::CodedInputStream input) {
    #if !GOOGLE_PROTOBUF_REFSTRUCT_COMPATIBILITY_MODE
      input.ReadRawMessage(this);
    #else
      uint tag;
      while ((tag = input.ReadTag()) != 0) {
        switch(tag) {
          default:
            _unknownFields = pb::UnknownFieldSet.MergeFieldFrom(_unknownFields, input);
            break;
          case 8: {
            Flags = (global::Amp.MediaFlags) input.ReadEnum();
            break;
          }
          case 26: {
            if (snapshotID_ == null) {
              SnapshotID = new global::Amp.Tag();
            }
            input.ReadMessage(SnapshotID);
            break;
          }
          case 41: {
            StartAt = input.ReadDouble();
            break;
          }
          case 49: {
            Seconds = input.ReadDouble();
            break;
          }
          case 133: {
            Popularity = input.ReadFloat();
            break;
          }
          case 141: {
            Ordering = input.ReadFloat();
            break;
          }
        }
      }
    #endif
    }

    #if !GOOGLE_PROTOBUF_REFSTRUCT_COMPATIBILITY_MODE
    [global::System.Diagnostics.DebuggerNonUserCodeAttribute]
    [global::System.CodeDom.Compiler.GeneratedCode("protoc", null)]
    void pb::IBufferMessage.InternalMergeFrom(ref pb::ParseContext input) {
      uint tag;
      while ((tag = input.ReadTag()) != 0) {
        switch(tag) {
          default:
            _unknownFields = pb::UnknownFieldSet.MergeFieldFrom(_unknownFields, ref input);
            break;
          case 8: {
            Flags = (global::Amp.MediaFlags) input.ReadEnum();
            break;
          }
          case 26: {
            if (snapshotID_ == null) {
              SnapshotID = new global::Amp.Tag();
            }
            input.ReadMessage(SnapshotID);
            break;
          }
          case 41: {
            StartAt = input.ReadDouble();
            break;
          }
          case 49: {
            Seconds = input.ReadDouble();
            break;
          }
          case 133: {
            Popularity = input.ReadFloat();
            break;
          }
          case 141: {
            Ordering = input.ReadFloat();
            break;
          }
        }
      }
    }
    #endif

  }

  #endregion

}

#endregion Designer generated code
