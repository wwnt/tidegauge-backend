# 数据项（item_name）说明

本文档面向**数据分析人员**。导出的数据**每个文件对应一条数据流**，文件名即 `item_name`，文件内每行只有 `timestamp(ms), value`。

下面按传感器型号列出**当前**所有的 `item_name` 及其含义。

## OTT PLS-C（压力式水文探头）

| `item_name`                              | 解释                            |
| ---------------------------------------- | ------------------------------- |
| `location1_water_level_pls_c`            | 水位（相对参考面），单位 m      |
| `location1_water_temperature`            | 水温，单位 ℃                    |
| `location1_water_conductivity`           | 电导率,单位 mS/cm               |
| `location1_water_salinity`               | 盐度，单位 PSU 或 ppt，取决于探头 `aOSS!` 配置（出厂默认 PSU） |
| `location1_water_total_dissolved_solids` | 溶解性总固体（TDS），单位 g/L。由探头根据电导率换算：数值 = TDS 系数 × 电导率[mS/cm]，系数出厂 0.64（`aOSF!` 可设 0.40–0.75），按海水环境标定 |

关于盐度与 TDS 的单位，以 OTT 手册为准，勿按字面照抄手册的公式标注：

- 盐度：PSU 模式显示 `0.00 … 42.00`（有效范围自 `2.00` 起），ppt 模式显示 `0.00 … 60.00`；出厂默认 PSU。
- TDS 的**输出**单位是 `g/l`（手册输出格式表：`TDS Measured value format: g/l ➝ pb.eee`）。
- 手册里那句 `TDS [ppm] = 0.64 × electrical conductivity [mS/cm]` 的**单位标注有误**：按海水 50 mS/cm 代入得 32，即 32 g/L（≈32000 mg/L），与真实海水 TDS 相符；若真按 32 ppm 理解则小了 1000 倍。因此该式数值上得到的是 **g/L**。

驱动侧对上述五项各有一个固定的合理性区间（越界即上报为空值，不按站可配）：水位 `[-5, 100]` m 覆盖 0…100 m 全部量程变体，电导率 `[0.005, 100]` mS/cm 覆盖 5 µS/cm…100 mS/cm，盐度/TDS `[0, 60]`；区间为闭区间，明细见 [config-reference.md](config-reference.md#ott-pls-c-plausibility-windows)。


## OTT SE200（浮子水位计）

| `item_name`                       | 解释                                         |
| --------------------------------- | -------------------------------------------- |
| `location1_water_level_shaft`     | 水位（浮子读数），单位 m               |

## VEGA VEGAPULS 61（雷达液位计）

| `item_name`                            | 解释                                   |
| -------------------------------------- | -------------------------------------- |
| `location1_radar_water_distance`       | 雷达天线到水面的距离，单位 m           |

## Vaisala HMP155（温湿度传感器）

| `item_name`                    | 解释                  |
| ------------------------------ | --------------------- |
| `location1_air_temperature`    | 气温，单位 ℃          |
| `location1_air_humidity`       | 相对湿度，单位 %RH    |

## Vaisala PTB330（数字气压计）

| `item_name`                 | 解释                  |
| --------------------------- | --------------------- |
| `location1_air_pressure`    | 大气压，单位 hPa      |

## Vaisala PWD50（能见度传感器）

| `item_name`                   | 解释                  |
| ----------------------------- | --------------------- |
| `location1_air_visibility`    | 能见度，单位 m        |

## Vaisala WMT700（超声波风速风向仪）

| `item_name`                    | 解释                    |
| ------------------------------ | ----------------------- |
| `location1_wind_speed`         | 风速，单位 m/s          |
| `location1_wind_direction`     | 风向，单位 °（度）      |

## Vaisala RG13（翻斗式雨量计）

| `item_name`     | 解释                                              |
| --------------- | ------------------------------------------------- |
| `rain_volume`   | 上报周期内累计降雨量（默认 1 分钟），单位 mm      |

## Vaisala DRD11A（雨水检测器，模拟电压输出）

| `item_name`            | 解释                                              |
| ---------------------- | ------------------------------------------------- |
| `drd11a_analog_out`    | DRD11A 模拟输出电压，单位 V；电压随降雨增大而下降 |
