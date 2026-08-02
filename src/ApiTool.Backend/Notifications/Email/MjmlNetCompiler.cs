using Mjml.Net;

namespace ApiTool.Backend.Notifications.Email;

/// <summary>
/// <see cref="IMjmlCompiler"/> implementation backed by the Mjml.Net 4.x managed library.
/// Compiles MJML in-process with no Node.js or native dependency — compatible with the
/// <c>aspnet:9.0-alpine</c> Docker runtime.
/// </summary>
public sealed class MjmlNetCompiler : IMjmlCompiler
{
    private readonly MjmlRenderer _renderer = new();

    /// <inheritdoc/>
    public string Compile(string mjml)
    {
        var (html, errors) = _renderer.Render(mjml, new MjmlOptions());
        if (errors.Count > 0)
            throw new InvalidOperationException(
                "MJML compile errors: " +
                string.Join("; ", errors.Select(e => $"{e.Type}:{e.Error}")));
        return html;
    }
}
