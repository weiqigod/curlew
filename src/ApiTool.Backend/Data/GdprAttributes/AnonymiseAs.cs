namespace ApiTool.Backend.Data.GdprAttributes;

/// <summary>Describes how a GDPR-tagged column should be anonymised on user deletion.</summary>
public enum AnonymiseAs
{
    /// <summary>
    /// Set the column to NULL when the schema allows it (nullable <c>Guid?</c> properties).
    /// Seven non-nullable <c>Guid</c> columns currently carry this declaration as forward
    /// intent — their schema widening is owed by M18-005 before M18-006 can honour it.
    /// See <c>GdprAttributeScannerTests.SetNull_on_nonnullable_Guid_columns_are_deferred_to_M18_005</c>
    /// for the canonical list.
    /// </summary>
    SetNull,

    /// <summary>Substitute the deterministic <c>deleted-user-{first8(sha256)}</c> token (for <c>string?</c> properties).</summary>
    DeletedUserToken,
}
