import 'package:flutter/material.dart';
import 'package:flutter/scheduler.dart';

/// Shared, short motion that follows the system's accessibility preferences.
abstract final class HhMotion {
  static const quick = Duration(milliseconds: 160);
  static const standard = Duration(milliseconds: 220);
  static const pressIn = Duration(milliseconds: 70);
  static const pressOut = Duration(milliseconds: 260);
  static const curve = Curves.easeOutCubic;

  /// 页面级滑动转场（设置页进入与其子页共用）：新页从左推入、原页右移。
  static const pageSlide = Duration(milliseconds: 340);
  static const pageSlideCurve = Cubic(.32, .72, .24, 1);

  // 按产品决策：动画不受任何门控（无障碍标记、系统减弱动画开关）影响，
  // 始终播放；保留方法与签名以兼容既有调用点。
  static bool reduced(BuildContext context) => false;

  static Duration duration(BuildContext context, Duration duration) =>
      reduced(context) ? Duration.zero : duration;
}

/// 微信式横向推入路由：新页从右缘纯位移滑入、覆盖当前页（无渐变/缩放），
/// 时长与曲线同 HhMotion.pageSlide；下层页可各自用 secondaryAnimation 做
/// 左移视差（见设置页的包装）。
class HhSlidePageRoute<T> extends PageRouteBuilder<T> {
  HhSlidePageRoute({required WidgetBuilder builder, super.settings})
    : super(
        transitionDuration: HhMotion.pageSlide,
        reverseTransitionDuration: HhMotion.pageSlide,
        pageBuilder: (context, animation, secondaryAnimation) =>
            builder(context),
        transitionsBuilder: (context, animation, secondaryAnimation, child) {
          final curved = CurvedAnimation(
            parent: animation,
            curve: HhMotion.pageSlideCurve,
            reverseCurve: HhMotion.pageSlideCurve,
          );
          return SlideTransition(
            position: curved.drive(
              Tween(begin: const Offset(1, 0), end: Offset.zero),
            ),
            child: child,
          );
        },
      );
}

/// Visual feedback driven by the button's own pointer and keyboard states.
///
/// Pass the supplied controller to the existing button's `statesController`.
/// This widget neither recognizes gestures nor invokes the button's action.
class HhPressFeedback extends StatefulWidget {
  const HhPressFeedback({
    super.key,
    required this.builder,
    this.enabled = true,
    this.pressedScale = .96,
  }) : assert(pressedScale > 0 && pressedScale <= 1);

  final Widget Function(BuildContext, WidgetStatesController) builder;
  final bool enabled;
  final double pressedScale;

  @override
  State<HhPressFeedback> createState() => _HhPressFeedbackState();
}

class _HhPressFeedbackState extends State<HhPressFeedback>
    with SingleTickerProviderStateMixin {
  final _states = WidgetStatesController();
  late final _scale = AnimationController.unbounded(vsync: this, value: 1);
  bool _reduced = false;
  bool _syncScheduled = false;
  double _targetScale = 1;

  @override
  void initState() {
    super.initState();
    _states.addListener(_statesChanged);
  }

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    _reduced = HhMotion.reduced(context);
    _syncScale();
  }

  @override
  void didUpdateWidget(HhPressFeedback oldWidget) {
    super.didUpdateWidget(oldWidget);
    _syncScale();
  }

  void _statesChanged() {
    // Native buttons can update disabled state while their subtree builds.
    if (SchedulerBinding.instance.schedulerPhase ==
        SchedulerPhase.persistentCallbacks) {
      if (_syncScheduled) return;
      _syncScheduled = true;
      WidgetsBinding.instance.addPostFrameCallback((_) {
        _syncScheduled = false;
        if (mounted) _syncScale();
      });
    } else {
      _syncScale();
    }
  }

  void _syncScale() {
    final pressed =
        widget.enabled &&
        !_states.value.contains(WidgetState.disabled) &&
        _states.value.contains(WidgetState.pressed);
    final target = pressed ? widget.pressedScale : 1.0;
    if (_reduced) {
      _scale.value = target;
    } else if (target != _targetScale) {
      _scale.animateTo(
        target,
        duration: pressed ? HhMotion.pressIn : HhMotion.pressOut,
        curve: HhMotion.curve,
      );
    }
    _targetScale = target;
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: _scale,
    child: widget.builder(context, _states),
    builder: (context, child) => Transform.scale(
      scale: _scale.value,
      transformHitTests: false,
      child: child,
    ),
  );

  @override
  void dispose() {
    _states.removeListener(_statesChanged);
    _states.dispose();
    _scale.dispose();
    super.dispose();
  }
}

/// A localized count transition; complete strings preserve shaping and RTL.
/// Only the current text is exposed to accessibility services.
class HhAnimatedCount extends StatefulWidget {
  const HhAnimatedCount({
    super.key,
    required this.text,
    this.style,
    this.textAlign = TextAlign.start,
  });

  final String text;
  final TextStyle? style;
  final TextAlign textAlign;

  @override
  State<HhAnimatedCount> createState() => _HhAnimatedCountState();
}

class _HhAnimatedCountState extends State<HhAnimatedCount>
    with SingleTickerProviderStateMixin {
  late final _progress = AnimationController(vsync: this, value: 1);
  late List<_CountLayer> _layers;
  bool _reduced = false;

  @override
  void initState() {
    super.initState();
    _layers = [_CountLayer.steady(widget.text)];
  }

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    _reduced = HhMotion.reduced(context);
    if (_reduced) {
      _layers = [_CountLayer.steady(widget.text)];
      _progress.value = 1;
    }
  }

  @override
  void didUpdateWidget(HhAnimatedCount oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (widget.text == oldWidget.text) return;
    if (_reduced) {
      _layers = [_CountLayer.steady(widget.text)];
      _progress.value = 1;
    } else {
      final progress = HhMotion.curve.transform(_progress.value);
      // Retarget the actual visible mixture, including an earlier value that
      // becomes current again. Resetting an old/new pair would flash the last
      // requested text at full opacity when a request is quickly rolled back.
      // Opacity rounds to 8-bit alpha; discard only fully invisible layers.
      _layers = [
        for (final layer in _layers)
          if (layer.opacityAt(progress) > .5 / 255)
            layer.retarget(progress, selected: layer.text == widget.text),
      ];
      if (!_layers.any((layer) => layer.text == widget.text)) {
        _layers.add(_CountLayer.incoming(widget.text));
      }
      _progress.duration = HhMotion.quick;
      _progress.forward(from: 0);
    }
  }

  @override
  Widget build(BuildContext context) {
    final style = DefaultTextStyle.of(context).style.merge(widget.style);
    Widget text(String value) => Text(
      value,
      style: style,
      textAlign: widget.textAlign,
      maxLines: 1,
      softWrap: false,
      overflow: TextOverflow.clip,
      strutStyle: StrutStyle.fromTextStyle(style, forceStrutHeight: true),
    );

    return Semantics(
      label: widget.text,
      textDirection: Directionality.of(context),
      child: ExcludeSemantics(
        child: AnimatedBuilder(
          animation: _progress,
          builder: (context, _) {
            final progress = HhMotion.curve.transform(_progress.value);
            return ClipRect(
              child: Stack(
                alignment: AlignmentDirectional.centerStart,
                children: [
                  // Preserve paint order as well as opacity and translation.
                  // Only the current string determines width and line height.
                  for (final layer in _layers)
                    if (layer.text == widget.text ||
                        layer.opacityAt(progress) > .5 / 255)
                      _countLayer(layer, progress, text(layer.text)),
                ],
              ),
            );
          },
        ),
      ),
    );
  }

  Widget _countLayer(_CountLayer layer, double progress, Widget text) {
    final child = FractionalTranslation(
      translation: Offset(0, layer.offsetAt(progress)),
      child: Opacity(opacity: layer.opacityAt(progress), child: text),
    );
    return layer.text == widget.text
        ? KeyedSubtree(key: ValueKey(layer.text), child: child)
        : Positioned.fill(key: ValueKey(layer.text), child: child);
  }

  @override
  void dispose() {
    _progress.dispose();
    super.dispose();
  }
}

class _CountLayer {
  const _CountLayer(
    this.text,
    this.startOpacity,
    this.startOffset,
    this.targetOpacity,
    this.targetOffset,
  );

  const _CountLayer.steady(String text) : this(text, 1, 0, 1, 0);
  const _CountLayer.incoming(String text) : this(text, 0, .16, 1, 0);

  final String text;
  final double startOpacity, startOffset, targetOpacity, targetOffset;

  double opacityAt(double progress) =>
      startOpacity + (targetOpacity - startOpacity) * progress;
  double offsetAt(double progress) =>
      startOffset + (targetOffset - startOffset) * progress;

  _CountLayer retarget(double progress, {required bool selected}) {
    final offset = offsetAt(progress);
    return _CountLayer(
      text,
      opacityAt(progress),
      offset,
      selected ? 1 : 0,
      selected ? 0 : offset - .16,
    );
  }
}

/// Interruptible 0–1 progress for an existing selected-state presentation.
class HhAnimatedSelection extends StatefulWidget {
  const HhAnimatedSelection({
    super.key,
    required this.selected,
    required this.builder,
  });

  final bool selected;
  final Widget Function(BuildContext, double) builder;

  @override
  State<HhAnimatedSelection> createState() => _HhAnimatedSelectionState();
}

class _HhAnimatedSelectionState extends State<HhAnimatedSelection>
    with SingleTickerProviderStateMixin {
  late final _progress = AnimationController(
    vsync: this,
    value: widget.selected ? 1 : 0,
  );
  bool _reduced = false;

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    _reduced = HhMotion.reduced(context);
    if (_reduced) _progress.value = widget.selected ? 1 : 0;
  }

  @override
  void didUpdateWidget(HhAnimatedSelection oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.selected == widget.selected) return;
    final target = widget.selected ? 1.0 : 0.0;
    if (_reduced) {
      _progress.value = target;
    } else {
      _progress.animateTo(
        target,
        duration: HhMotion.standard,
        curve: HhMotion.curve,
      );
    }
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: _progress,
    builder: (context, _) => widget.builder(context, _progress.value),
  );

  @override
  void dispose() {
    _progress.dispose();
    super.dispose();
  }
}
