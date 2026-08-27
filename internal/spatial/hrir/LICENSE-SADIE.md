# SADIE II attribution

`sadie48.bin` contains head-related impulse responses from the **SADIE II
Database**, licensed **CC BY 4.0**. That licence is compatible with this
repository's MIT licence and requires attribution, which is what this file is.

## Source

> Armstrong, C., Thresh, L., Murphy, D., & Kearney, G. (2018).
> *A Perceptual Evaluation of Individual and Non-Individual HRTFs: A Case Study
> of the SADIE II Database.* Applied Sciences, 8(11), 2029.
> https://doi.org/10.3390/app8112029

Database: <https://www.york.ac.uk/sadie-project/database.html>
Deposit: <https://zenodo.org/records/12092466>
Licence: <https://creativecommons.org/licenses/by/4.0/>

## What was taken

Subject **D1**, a Neumann KU100 dummy head, from
`D1_48K_24bit_256tap_FIR_SOFA.sofa`: 48 kHz, 256 taps, both ears.

Of the 8802 measured directions, **thirteen** are embedded, one per speaker
position across occam's layouts:

| azimuth | elevation | used by |
| --- | --- | --- |
| 0 | 0 | centre |
| ±30 | 0 | front left and right |
| ±90 | 0 | side surrounds |
| ±110 | 0 | 5.1 surrounds |
| ±150 | 0 | rear surrounds |
| ±45 | 45 | top front |
| ±135 | 45 | top rear |

Each requested angle matched a measured position exactly, so none of the
responses are interpolated or otherwise altered. Samples were converted from
float64 to float32; nothing else was changed.

Azimuth here is degrees clockwise from front, so +90 is to the listener's
right. SADIE's own convention is counter-clockwise, and the extraction negates
it on the way in.

## Regenerating

`just hrir` re-extracts the blob from a downloaded SOFA file. The recipe prints
the download URL and shows the angular error for every direction, which should
read 0.00 throughout.
