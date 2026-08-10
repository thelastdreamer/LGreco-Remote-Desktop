using System;
using System.Runtime.InteropServices;
using System.Windows;
using System.Windows.Controls;
using System.Windows.Media;
using System.Windows.Media.Imaging;

namespace RemoteDesktop.Client.Controls;

public partial class RemoteViewer : UserControl
{
    private WriteableBitmap? _bitmap;
    private IntPtr _backBuffer;
    private int _width = 1280;
    private int _height = 720;

    public RemoteViewer()
    {
        InitializeComponent();
    }

    public void RenderFrame(byte[] data, int width, int height)
    {
        Dispatcher.Invoke(() =>
        {
            if (_bitmap == null || width != _width || height != _height)
            {
                _width = width;
                _height = height;
                _bitmap = new WriteableBitmap(
                    width, height, 96, 96, PixelFormats.Bgra32, null);
                _bitmap.Lock();
                _backBuffer = _bitmap.BackBuffer;
                VideoImage.Source = _bitmap;
            }
            else
            {
                _bitmap.Lock();
                _backBuffer = _bitmap.BackBuffer;
            }

            if (data.Length >= _width * _height * 4)
            {
                Marshal.Copy(data, 0, _backBuffer, _width * _height * 4);
            }

            _bitmap.AddDirtyRect(new Int32Rect(0, 0, _width, _height));
            _bitmap.Unlock();

            PlaceholderText.Visibility = Visibility.Collapsed;
        });
    }

    public void ClearFrame()
    {
        Dispatcher.Invoke(() =>
        {
            _bitmap = null;
            VideoImage.Source = null;
            PlaceholderText.Visibility = Visibility.Visible;
        });
    }
}
