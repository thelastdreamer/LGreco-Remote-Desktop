using System;
using System.Net.Http;
using System.Net.Http.Headers;
using System.Text;
using System.Threading.Tasks;
using Newtonsoft.Json;
using RemoteDesktop.Client.Models;

namespace RemoteDesktop.Client.Services;

public class ApiClient
{
    private readonly HttpClient _http;
    private string _token = "";

    public ApiClient()
    {
        _http = new HttpClient { Timeout = TimeSpan.FromSeconds(15) };
    }

    public void SetToken(string token)
    {
        _token = token;
        _http.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", token);
    }

    public async Task<(UserInfo? user, string? error)> LoginAsync(string serverUrl, string username, string password)
    {
        try
        {
            var payload = new { username, password };
            var content = new StringContent(
                JsonConvert.SerializeObject(payload), Encoding.UTF8, "application/json");
            var response = await _http.PostAsync($"{serverUrl}/api/login", content);

            if (!response.IsSuccessStatusCode)
            {
                var errBody = await response.Content.ReadAsStringAsync();
                var apiErr = JsonConvert.DeserializeObject<ApiError>(errBody);
                return (null, apiErr?.Error ?? "Login failed");
            }

            var body = await response.Content.ReadAsStringAsync();
            var loginResp = JsonConvert.DeserializeAnonymousType(body, new { token = "", user = new UserInfo() });
            if (loginResp == null)
                return (null, "Invalid response");

            SetToken(loginResp.token);
            return (loginResp.user, null);
        }
        catch (Exception ex)
        {
            return (null, $"Connection error: {ex.Message}");
        }
    }

    public async Task<(UserInfo? user, string? error)> GetMeAsync(string serverUrl)
    {
        try
        {
            var response = await _http.GetAsync($"{serverUrl}/api/me");
            if (!response.IsSuccessStatusCode)
                return (null, "Not authenticated");
            var body = await response.Content.ReadAsStringAsync();
            var user = JsonConvert.DeserializeObject<UserInfo>(body);
            return (user, null);
        }
        catch (Exception ex)
        {
            return (null, $"Error: {ex.Message}");
        }
    }

    public async Task<(SessionInfo? session, string? error)> CreateSessionAsync(
        string serverUrl, string type = "desktop", string resolution = "1280x720",
        bool audio = true, string? targetHost = null, int targetPort = 3389)
    {
        try
        {
            var payload = new
            {
                type,
                resolution,
                audio_enabled = audio,
                clipboard_sync = true,
                target_host = targetHost ?? "",
                target_port = targetPort,
            };
            var content = new StringContent(
                JsonConvert.SerializeObject(payload), Encoding.UTF8, "application/json");
            var response = await _http.PostAsync($"{serverUrl}/api/sessions", content);

            if (!response.IsSuccessStatusCode)
            {
                var errBody = await response.Content.ReadAsStringAsync();
                var apiErr = JsonConvert.DeserializeObject<ApiError>(errBody);
                return (null, apiErr?.Error ?? "Failed to create session");
            }

            var body = await response.Content.ReadAsStringAsync();
            var sessionResp = JsonConvert.DeserializeAnonymousType(body,
                new { session = new SessionInfo(), signal_url = "", ice_servers = Array.Empty<IceServer>() });

            if (sessionResp?.session == null)
                return (null, "Invalid response");

            sessionResp.session.SignalUrl = sessionResp.signal_url;
            sessionResp.session.IceServers = sessionResp.ice_servers;
            return (sessionResp.session, null);
        }
        catch (Exception ex)
        {
            return (null, $"Error: {ex.Message}");
        }
    }

    public async Task<(SessionInfo[]? sessions, string? error)> ListSessionsAsync(string serverUrl)
    {
        try
        {
            var response = await _http.GetAsync($"{serverUrl}/api/sessions");
            if (!response.IsSuccessStatusCode)
                return (null, "Failed to list sessions");
            var body = await response.Content.ReadAsStringAsync();
            var sessions = JsonConvert.DeserializeObject<SessionInfo[]>(body);
            return (sessions, null);
        }
        catch (Exception ex)
        {
            return (null, $"Error: {ex.Message}");
        }
    }

    public async Task<string?> DeleteSessionAsync(string serverUrl, long sessionId)
    {
        try
        {
            var response = await _http.DeleteAsync($"{serverUrl}/api/sessions/{sessionId}");
            if (!response.IsSuccessStatusCode)
                return "Failed to delete session";
            return null;
        }
        catch (Exception ex)
        {
            return $"Error: {ex.Message}";
        }
    }
}
