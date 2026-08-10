using System;
using System.Threading.Tasks;
using System.Windows;
using System.Windows.Controls;
using System.Windows.Input;
using System.Windows.Media;
using System.Windows.Shapes;
using RemoteDesktop.Client.Models;
using RemoteDesktop.Client.Services;

namespace RemoteDesktop.Client;

public partial class MainWindow : Window
{
    private readonly ApiClient _api = new();
    private readonly BridgeClient _bridge = new();
    private InputForwarder? _inputForwarder;
    private ConnectionInfo _connection = new();
    private SessionInfo? _currentSession;

    public MainWindow()
    {
        InitializeComponent();
        _bridge.OnStatusChanged += status =>
            Dispatcher.Invoke(() => UpdateStatus(status));
    }

    private void UpdateStatus(string status)
    {
        ConnectionStatus.Text = status;
        var isConnected = status.Contains("Connected");
        StatusDot.Fill = new SolidColorBrush(
            isConnected ? Color.FromRgb(0x4E, 0xCD, 0xC4) : Color.FromRgb(0xFF, 0x6B, 0x6B));
    }

    private void TitleBar_MouseDown(object sender, MouseButtonEventArgs e)
    {
        if (e.ChangedButton == MouseButton.Left)
            DragMove();
    }

    private void CloseButton_Click(object sender, RoutedEventArgs e)
    {
        _bridge.Stop();
        Application.Current.Shutdown();
    }

    private async void LoginButton_Click(object sender, RoutedEventArgs e)
    {
        LoginButton.IsEnabled = false;
        LoginError.Visibility = Visibility.Collapsed;

        _connection.ServerUrl = ServerUrlBox.Text.Trim();
        _connection.Username = UsernameBox.Text.Trim();
        _connection.Password = PasswordBox.Password;

        var (user, error) = await _api.LoginAsync(
            _connection.ServerUrl, _connection.Username, _connection.Password);

        if (error != null)
        {
            LoginError.Text = error;
            LoginError.Visibility = Visibility.Visible;
            LoginButton.IsEnabled = true;
            return;
        }

        _connection.IsLoggedIn = true;
        _connection.Username = user!.Username;
        LoginPanel.Visibility = Visibility.Collapsed;
        MainPanel.Visibility = Visibility.Visible;
        StatusText.Text = $"Logged in as {user.Username}";
        ConnectionStatus.Text = "Connected to server";
        StatusDot.Fill = new SolidColorBrush(Color.FromRgb(0x4E, 0xCD, 0xC4));
        LoginButton.IsEnabled = true;

        await RefreshSessionsAsync();
    }

    private void BtnDisconnect_Click(object sender, RoutedEventArgs e)
    {
        DisconnectCurrentSession();

        _connection = new ConnectionInfo();
        LoginPanel.Visibility = Visibility.Visible;
        MainPanel.Visibility = Visibility.Collapsed;
    }

    private void DisconnectCurrentSession()
    {
        if (_currentSession != null)
        {
            _bridge.Stop();
            _inputForwarder?.Deactivate(RemoteViewerControl);
            _currentSession = null;
            RemoteViewerControl.ClearFrame();
            SessionInfoBar.Visibility = Visibility.Collapsed;
        }
        UpdateStatus("Disconnected");
    }

    private async void BtnNewDesktop_Click(object sender, RoutedEventArgs e)
    {
        await CreateSessionAsync("desktop");
    }

    private void BtnNewRelay_Click(object sender, RoutedEventArgs e)
    {
        var dialog = new Window
        {
            Title = "New Relay Session",
            Width = 350, Height = 180,
            WindowStartupLocation = WindowStartupLocation.CenterOwner,
            Owner = this,
            ResizeMode = ResizeMode.NoResize,
            WindowStyle = WindowStyle.ToolWindow,
            Background = new SolidColorBrush(Color.FromRgb(0x1E, 0x1E, 0x2E)),
            Foreground = new SolidColorBrush(Color.FromRgb(0xE0, 0xE0, 0xF0)),
        };
        var sp = new StackPanel { Margin = new Thickness(16, 12, 16, 8) };
        sp.Children.Add(new TextBlock
        {
            Text = "Target RDP Host:", Foreground = new SolidColorBrush(Colors.Gray),
            Margin = new Thickness(0, 0, 0, 4)
        });
        var input = new TextBox { Text = "192.168.1.100", Margin = new Thickness(0, 0, 0, 12),
            Background = new SolidColorBrush(Color.FromRgb(0x2D, 0x2D, 0x3F)),
            Foreground = new SolidColorBrush(Color.FromRgb(0xE0, 0xE0, 0xF0)),
            BorderBrush = new SolidColorBrush(Color.FromRgb(0x3D, 0x3D, 0x55)) };
        sp.Children.Add(input);
        var btnPanel = new StackPanel { Orientation = Orientation.Horizontal, HorizontalAlignment = HorizontalAlignment.Right };
        var cancel = new Button { Content = "Cancel", Width = 70, Height = 28, Margin = new Thickness(0, 0, 8, 0) };
        var ok = new Button { Content = "Connect", Width = 70, Height = 28 };
        cancel.Click += (_, _) => dialog.DialogResult = false;
        ok.Click += (_, _) => dialog.DialogResult = true;
        btnPanel.Children.Add(cancel);
        btnPanel.Children.Add(ok);
        sp.Children.Add(btnPanel);
        dialog.Content = sp;

        if (dialog.ShowDialog() != true) return;
        var host = input.Text.Trim();
        if (string.IsNullOrWhiteSpace(host)) return;

        _ = CreateSessionAsync("relay", host);
    }


    private async Task CreateSessionAsync(string type, string targetHost = "")
    {
        BtnNewDesktop.IsEnabled = false;
        BtnNewRelay.IsEnabled = false;
        StatusText.Text = "Creating session...";

        var (session, error) = await _api.CreateSessionAsync(
            _connection.ServerUrl, type, targetHost: targetHost);

        if (error != null)
        {
            StatusText.Text = $"Error: {error}";
            BtnNewDesktop.IsEnabled = true;
            BtnNewRelay.IsEnabled = true;
            return;
        }

        _currentSession = session!;
        StatusText.Text = $"Session {session!.Id} created";
        ResolutionText.Text = session.Resolution;
        SessionIdText.Text = $"Session #{session.Id}";
        SessionInfoBar.Visibility = Visibility.Visible;

        ConnectToSession(session);
        await RefreshSessionsAsync();

        BtnNewDesktop.IsEnabled = true;
        BtnNewRelay.IsEnabled = true;
    }

    private void ConnectToSession(SessionInfo session)
    {
        DisconnectCurrentSession();
        _currentSession = session;

        _bridge.Start(_connection.ServerUrl, session.Id.ToString(), _connection.Token);

        _inputForwarder = new InputForwarder(async cmd =>
        {
            if (cmd != null)
                await _bridge.SendCommandAsync(cmd);
        });
        _inputForwarder.Activate(RemoteViewerControl);

        UpdateStatus($"Connected to session {session.Id}");
        StatusText.Text = $"Session {session.Id} active";
        SessionInfoBar.Visibility = Visibility.Visible;
    }

    private async Task RefreshSessionsAsync()
    {
        var (sessions, error) = await _api.ListSessionsAsync(_connection.ServerUrl);
        if (error != null || sessions == null) return;

        SessionList.Children.Clear();
        foreach (var s in sessions)
        {
            var isActive = _currentSession?.Id == s.Id;
            var bg = isActive ? "#6C63FF22" : "Transparent";
            var fg = isActive ? "#6C63FF" : "#888";

            var btn = new Button
            {
                Width = 48, Height = 48,
                Margin = new Thickness(0, 0, 0, 6),
                Background = new SolidColorBrush((Color)ColorConverter.ConvertFromString(bg)),
                BorderThickness = new Thickness(0),
                Cursor = Cursors.Hand,
                ToolTip = $"[{s.Type}] Session {s.Id}: {s.Status}",
                Tag = s,
                Content = new StackPanel
                {
                    Children = {
                        new TextBlock
                        {
                            Text = s.Type == "desktop" ? "PC" : "RL",
                            FontSize = 12, FontWeight = FontWeights.Bold,
                            Foreground = new SolidColorBrush((Color)ColorConverter.ConvertFromString(fg)),
                            HorizontalAlignment = HorizontalAlignment.Center,
                        },
                        new TextBlock
                        {
                            Text = s.Status.Substring(0, Math.Min(s.Status.Length, 3)).ToUpper(),
                            FontSize = 8,
                            Foreground = new SolidColorBrush(Colors.Gray),
                            HorizontalAlignment = HorizontalAlignment.Center,
                        }
                    }
                }
            };

            btn.Click += (_, _) =>
            {
                _currentSession = s;
                ConnectToSession(s);
                ResolutionText.Text = s.Resolution;
                SessionIdText.Text = $"Session #{s.Id}";
                StatusText.Text = $"Connected to session {s.Id}";
                _ = RefreshSessionsAsync();
            };

            SessionList.Children.Add(btn);
        }
    }

    protected override void OnClosed(EventArgs e)
    {
        _bridge.Stop();
        base.OnClosed(e);
    }
}
