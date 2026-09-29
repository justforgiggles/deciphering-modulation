# FDM test photos

Downloaded as 128×128 JPEGs on 2026-09-29 through [Lorem Picsum](https://picsum.photos).
Small images keep the test WAV manageable at 100 bits/second per channel.

| Local file | Photographer | Original | Download |
|---|---|---|---|
| stock-landscape.jpg | Paul Jarvis | https://unsplash.com/photos/6J--NXulQCs | https://picsum.photos/id/10/128/128 |
| stock-dog.jpg | André Spieker | https://unsplash.com/photos/8wTPqxlnKM4 | https://picsum.photos/id/237/128/128 |

Repeat the test with:

```sh
./run_fdm.sh data/stock-landscape.jpg data/stock-dog.jpg am 1070 3070
```
