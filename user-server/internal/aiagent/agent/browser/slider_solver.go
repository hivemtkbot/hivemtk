package browser

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"time"

	"marketing/internal/pkg/utils/logger"
)

// SliderConfig 滑块配置
type SliderConfig struct {
	MaxRetries      int     // 最大重试次数
	MinSpeed        float64 // 最小拖动速度 (px/s)
	MaxSpeed        float64 // 最大拖动速度 (px/s)
	OvershootRatio  float64 // 过冲比例 (会多滑动一点再回退)
	JitterAmplitude float64 // Y轴抖动幅度 (px)
	StepCount       int     // 轨迹步数
}

// DefaultSliderConfig 默认滑块配置
var DefaultSliderConfig = SliderConfig{
	MaxRetries:      3,
	MinSpeed:        100.0,
	MaxSpeed:        300.0,
	OvershootRatio:  0.05,
	JitterAmplitude: 3.0,
	StepCount:       30,
}

// SliderSolver 滑块验证码求解器
type SliderSolver struct {
	config SliderConfig
}

// NewSliderSolver 创建滑块求解器
func NewSliderSolver(config SliderConfig) *SliderSolver {
	return &SliderSolver{config: config}
}

// TrajectoryPoint 轨迹点
type TrajectoryPoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	T int64   `json:"t"` // 相对时间 (ms)
}

// GenerateTrajectory 生成 Perlin 噪声 + 贝塞尔曲线的拟人滑动轨迹
//
// 原理：
//  1. 人类拖拽滑块的轨迹不是直线的
//  2. 速度分布：慢 → 快 → 慢（加速-匀速-减速）
//  3. Y 轴有小幅抖动（不是完美水平）
//  4. 末尾有过冲和微调（overshoot + fine-tune）
//
// 参考：dengyie/xianyu-auto-bot 的 slider_trajectory.py
func (s *SliderSolver) GenerateTrajectory(distance float64) []TrajectoryPoint {
	steps := s.config.StepCount
	points := make([]TrajectoryPoint, 0, steps)

	// 1. Perlin-like 噪声生成 X 轴位移
	// 使用正弦波叠加模拟自然的不规则运动
	totalTime := 0.0
	overshoot := distance * (1.0 + s.config.OvershootRatio)

	for i := 0; i < steps; i++ {
		// 进度: 0.0 -> 1.0
		progress := float64(i) / float64(steps)

		// 使用 easeOutCubic 曲线模拟加速 → 减速
		// f(t) = 1 - (1-t)³
		easedProgress := 1.0 - math.Pow(1.0-progress, 3)

		// X 位移 = 基础位移 + 微调噪声
		noiseFactor := 0.02 // 噪声幅度
		noise := math.Sin(progress*math.Pi*float64(3+rand.Intn(2))) * noiseFactor * distance
		x := overshoot*easedProgress + noise

		// Y 轴抖动 (小幅度，模拟手抖)
		y := math.Sin(progress*math.Pi*float64(4+rand.Intn(4))) *
			s.config.JitterAmplitude * (1.0 - math.Abs(progress-0.5)*2.0)

		// 时间增量 (模拟速度变化)
		// 开始慢，中间快，结尾慢
		speedFactor := 1.0 + math.Sin(progress*math.Pi)*0.5 // 0.5 ~ 1.5 波动
		stepDuration := (distance / float64(steps)) / (s.config.MinSpeed + (s.config.MaxSpeed-s.config.MinSpeed)*speedFactor)
		stepDuration *= 1000.0 // 转毫秒

		// 6% 概率随机微停顿 (模拟犹豫)
		if rand.Float64() < 0.06 {
			pauseMs := float64(rand.Intn(200) + 50)
			totalTime += pauseMs
		}

		totalTime += math.Max(stepDuration, 5.0) // 最少 5ms

		points = append(points, TrajectoryPoint{
			X: math.Max(x, 0),
			Y: y,
			T: int64(totalTime),
		})
	}

	// 2. 最后的微调阶段 (fine-tune)
	// 到达末尾后可能小幅回退再前进
	fineSteps := 3 + rand.Intn(3)
	currentX := overshoot
	for i := 0; i < fineSteps; i++ {
		// 小幅回退
		adjustX := currentX - float64(rand.Intn(5))
		totalTime += float64(rand.Intn(100) + 50)

		points = append(points, TrajectoryPoint{
			X: math.Max(adjustX, 0),
			Y: float64(rand.Intn(2)),
			T: int64(totalTime),
		})

		// 再次前进到最终位置
		currentX = overshoot + float64(rand.Intn(3))
		totalTime += float64(rand.Intn(100) + 50)

		points = append(points, TrajectoryPoint{
			X: math.Min(currentX, distance*1.1),
			Y: float64(rand.Intn(2)),
			T: int64(totalTime),
		})
	}

	return points
}

// trajectoryToJS 将轨迹转为 JS 可执行代码
func trajectoryToJS(points []TrajectoryPoint) string {
	if len(points) == 0 {
		return ""
	}

	js := `(function(){
        var slider = document.querySelector('.slider,.nc_iconfont.btn_slide,.slidetrack,[class*="slider"],.xianyu-slider');
        if(!slider) return JSON.stringify({ok:false, reason:'slider_not_found'});

        var rect = slider.getBoundingClientRect();
        var startX = rect.left + rect.width/2;
        var startY = rect.top + rect.height/2;
        var track = slider.parentElement;
        var trackRect = track.getBoundingClientRect();
        var maxX = trackRect.right - rect.width/2;

        // 定义轨迹点
        var points = %s;

        // 模拟 mousedown
        slider.dispatchEvent(new MouseEvent('mousedown', {
            clientX: startX, clientY: startY, bubbles: true
        }));

        // 逐点移动
        function moveTo(index) {
            if(index >= points.length) {
                // mouseup at last position
                var last = points[points.length-1];
                slider.dispatchEvent(new MouseEvent('mouseup', {
                    clientX: Math.min(startX + last.x, maxX),
                    clientY: startY + last.y,
                    bubbles: true
                }));
                return JSON.stringify({ok:true, points:points.length});
            }

            var p = points[index];
            var newX = Math.min(startX + p.x, maxX);
            var newY = startY + p.y;

            slider.dispatchEvent(new MouseEvent('mousemove', {
                clientX: newX, clientY: newY, bubbles: true
            }));

            // 按轨迹时间间隔调用下一步
            var nextDelay = index > 0 ? (p.t - points[index-1].t) : p.t;
            setTimeout(function(){ moveTo(index+1); }, Math.max(nextDelay, 10));
        }

        moveTo(0);
        return JSON.stringify({ok:true, started:true, points:points.length});
    })();`

	// 序列化轨迹点
	pointsJSON, _ := json.Marshal(points)
	return fmt.Sprintf(js, string(pointsJSON))
}

// Solve 求解滑块验证码
func (s *SliderSolver) Solve(bot *AutoReplyBot) error {
	for attempt := 1; attempt <= s.config.MaxRetries; attempt++ {
		logger.Infof("[%s] 滑块验证码尝试 %d/%d", bot.platform, attempt, s.config.MaxRetries)

		// 1. 检测滑块是否存在
		js := `(function(){
            var el = document.querySelector('.slider,.nc_iconfont.btn_slide,.slidetrack,[class*="slider"],.xianyu-slider');
            if(!el) return JSON.stringify({exists:false});
            var rect = el.getBoundingClientRect();
            return JSON.stringify({exists:true, width:rect.width, height:rect.height});
        })();`

		result, err := bot.assistant.Evaluate(js)
		if err != nil {
			return fmt.Errorf("检测滑块失败: %w", err)
		}

		var check struct {
			Exists bool    `json:"exists"`
			Width  float64 `json:"width"`
			Height float64 `json:"height"`
		}
		if err := json.Unmarshal([]byte(result), &check); err != nil {
			return fmt.Errorf("解析滑块检测结果失败: %w", err)
		}

		if !check.Exists {
			logger.Infof("[%s] 没有检测到滑块验证码", bot.platform)
			return nil
		}

		// 2. 获取滑块需要移动的距离
		// 距离 = 轨道宽度 - 滑块宽度 - padding
		distance := s.estimateDistance(bot.assistant)
		if distance <= 0 {
			distance = 260 // 默认距离
		}

		// 3. 生成拟人轨迹
		trajectory := s.GenerateTrajectory(distance)
		logger.Infof("[%s] 生成轨迹: %d 点, 总距离: %.0fpx", bot.platform, len(trajectory), distance)

		// 4. 执行拖动
		js2 := trajectoryToJS(trajectory)
		result2, err := bot.assistant.Evaluate(js2)
		if err != nil {
			logger.Errorf("[%s] 滑块执行失败: %v", bot.platform, err)
			continue
		}

		logger.Infof("[%s] 滑块结果: %s", bot.platform, result2)

		// 5. 等待验证结果
		time.Sleep(2 * time.Second)

		// 6. 检查滑块是否消失
		js3 := `(function(){
            var el = document.querySelector('.slider,.nc_iconfont.btn_slide,.slidetrack,[class*="slider"]');
            return el ? 'still_exists' : 'gone';
        })();`
		result3, _ := bot.assistant.Evaluate(js3)
		if result3 == "gone" {
			logger.Infof("[%s] 滑块验证码已通过", bot.platform)
			return nil
		}

		logger.Infof("[%s] 滑块仍在，进入下一次尝试", bot.platform)
		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("滑块验证码未通过，已重试 %d 次", s.config.MaxRetries)
}

// estimateDistance 估算滑块需要移动的距离
func (s *SliderSolver) estimateDistance(assistant *Assistant) float64 {
	js := `(function(){
        var track = document.querySelector('.slidetrack,.nc_scale,.slider-track,[class*="slidetrack"],[class*="slider-track"]');
        var slider = document.querySelector('.slider,.nc_iconfont.btn_slide,[class*="slider-btn"]');
        if(!track || !slider) return '-1';

        var trackRect = track.getBoundingClientRect();
        var sliderRect = slider.getBoundingClientRect();
        // 距离 = 轨道宽度 - 滑块宽度 - 10px padding
        var distance = trackRect.width - sliderRect.width - 20;
        return String(Math.max(distance, 0));
    })();`

	result, err := assistant.Evaluate(js)
	if err != nil || result == "-1" {
		return -1
	}

	var dist float64
	if _, err := fmt.Sscanf(result, "%f", &dist); err != nil {
		return -1
	}
	return dist
}
