package au.fintex.mnet;

import android.graphics.Bitmap;
import java.io.ByteArrayOutputStream;
import java.nio.ByteBuffer;

// A frame owns temporary native bitmap storage only for this call. The caller
// still owns the Image and must close it, including when encoding fails.
final class CaptureEncoder {
    private CaptureEncoder() {}
    static byte[] jpeg(ByteBuffer pixels, int width, int height, int pixelStride, int rowStride) {
        if (width <= 0 || height <= 0 || pixelStride != 4 || rowStride < (long) width * 4 || rowStride % 4 != 0) {
            throw new IllegalArgumentException("Unsupported capture frame layout");
        }
        Bitmap padded = null;
        Bitmap cropped = null;
        try {
            padded = Bitmap.createBitmap(rowStride / 4, height, Bitmap.Config.ARGB_8888);
            padded.copyPixelsFromBuffer(pixels);
            cropped = Bitmap.createBitmap(padded, 0, 0, width, height);
            ByteArrayOutputStream output = new ByteArrayOutputStream();
            if (!cropped.compress(Bitmap.CompressFormat.JPEG, 70, output)) {
                throw new IllegalStateException("Capture frame encoding failed");
            }
            return output.toByteArray();
        } finally {
            if (cropped != null && cropped != padded) cropped.recycle();
            if (padded != null) padded.recycle();
        }
    }
}
