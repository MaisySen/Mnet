package au.fintex.mnet;

import android.graphics.Bitmap;
import java.io.ByteArrayOutputStream;
import java.nio.ByteBuffer;

// Copy only visible RGBA bytes. Image.Plane does not promise trailing padding
// after the last row. A duplicate preserves the caller's buffer cursor/limit.
final class CaptureEncoder {
    private CaptureEncoder() {}
    static byte[] jpeg(ByteBuffer pixels, int width, int height, int pixelStride, int rowStride) {
        long rowBytes = (long) width * 4;
        long packedBytes = rowBytes * height;
        long required = (long) rowStride * (height - 1) + rowBytes;
        if (width <= 0 || height <= 0 || pixelStride != 4 || rowStride < rowBytes
                || rowStride % 4 != 0 || packedBytes > Integer.MAX_VALUE
                || required > pixels.remaining()) {
            throw new IllegalArgumentException("Unsupported capture frame layout");
        }
        ByteBuffer source = pixels.duplicate();
        ByteBuffer packed;
        if (rowStride == rowBytes) {
            source.limit(source.position() + (int) packedBytes);
            packed = source;
        } else {
            packed = ByteBuffer.allocate((int) packedBytes);
            int start = source.position();
            for (int y = 0; y < height; y++) {
                int offset = (int) (start + (long) y * rowStride);
                source.limit(pixels.limit());
                source.position(offset);
                source.limit(offset + (int) rowBytes);
                packed.put(source);
            }
            packed.flip();
        }
        Bitmap frame = null;
        try {
            frame = Bitmap.createBitmap(width, height, Bitmap.Config.ARGB_8888);
            frame.copyPixelsFromBuffer(packed);
            ByteArrayOutputStream output = new ByteArrayOutputStream();
            if (!frame.compress(Bitmap.CompressFormat.JPEG, 70, output)) {
                throw new IllegalStateException("Capture frame encoding failed");
            }
            return output.toByteArray();
        } finally {
            if (frame != null) frame.recycle();
        }
    }
}
