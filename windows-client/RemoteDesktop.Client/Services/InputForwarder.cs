using System;
using System.Windows;
using System.Windows.Input;
using RemoteDesktop.Client.Native;

namespace RemoteDesktop.Client.Services;

public class InputForwarder
{
    private readonly Func<object, Task> _sendCommand;
    private bool _isActive;

    public InputForwarder(Func<object, Task> sendCommand)
    {
        _sendCommand = sendCommand;
    }

    public void Activate(FrameworkElement element)
    {
        _isActive = true;

        element.KeyDown += OnKeyDown;
        element.KeyUp += OnKeyUp;
        element.MouseMove += OnMouseMove;
        element.MouseDown += OnMouseDown;
        element.MouseUp += OnMouseUp;
        element.MouseWheel += OnMouseWheel;
        element.Focusable = true;
        element.Focus();
    }

    public void Deactivate(FrameworkElement element)
    {
        _isActive = false;

        element.KeyDown -= OnKeyDown;
        element.KeyUp -= OnKeyUp;
        element.MouseMove -= OnMouseMove;
        element.MouseDown -= OnMouseDown;
        element.MouseUp -= OnMouseUp;
        element.MouseWheel -= OnMouseWheel;
    }

    private async void OnKeyDown(object sender, KeyEventArgs e)
    {
        if (!_isActive) return;
        await _sendCommand(new
        {
            cmd = 3,
            data = new { type = "keydown", keycode = KeyInterop.VirtualKeyFromKey(e.Key) }
        });
    }

    private async void OnKeyUp(object sender, KeyEventArgs e)
    {
        if (!_isActive) return;
        await _sendCommand(new
        {
            cmd = 3,
            data = new { type = "keyup", keycode = KeyInterop.VirtualKeyFromKey(e.Key) }
        });
    }

    private async void OnMouseMove(object sender, MouseEventArgs e)
    {
        if (!_isActive) return;
        var pos = e.GetPosition((IInputElement)sender);
        await _sendCommand(new
        {
            cmd = 3,
            data = new { type = "mousemove", x = pos.X, y = pos.Y }
        });
    }

    private async void OnMouseDown(object sender, MouseButtonEventArgs e)
    {
        if (!_isActive) return;
        await _sendCommand(new
        {
            cmd = 3,
            data = new { type = "mousedown", button = (int)e.ChangedButton }
        });
    }

    private async void OnMouseUp(object sender, MouseButtonEventArgs e)
    {
        if (!_isActive) return;
        await _sendCommand(new
        {
            cmd = 3,
            data = new { type = "mouseup", button = (int)e.ChangedButton }
        });
    }

    private async void OnMouseWheel(object sender, MouseWheelEventArgs e)
    {
        if (!_isActive) return;
        await _sendCommand(new
        {
            cmd = 3,
            data = new { type = "wheel", delta = e.Delta }
        });
    }
}
