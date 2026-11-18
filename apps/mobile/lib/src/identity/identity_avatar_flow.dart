import 'dart:async';
import 'dart:math' as math;
import 'dart:typed_data';
import 'dart:ui' as ui;

import 'package:flutter/material.dart';

import '../design_system/telegram_theme.dart';
import '../localization/app_localizations.dart';
import '../media/album_photo_picker.dart';

const _cropRadiusFraction = .34;

enum _CropAction { chooseAnother }

/// 头像流程的短暂失败提示：3 秒自动消失，可点「关闭」立即关闭。
/// 先清掉残留旧通知，避免排队叠加造成“不消失”的观感。
void notifyAvatarBlocked(BuildContext context, String message) {
  final messenger = ScaffoldMessenger.of(context);
  messenger.hideCurrentSnackBar();
  messenger.showSnackBar(
    SnackBar(
      content: Text(hhText(context, message)),
      duration: const Duration(seconds: 3),
      behavior: SnackBarBehavior.floating,
      // 本 SDK（3.50 预览版）中带 action 的 SnackBar persist 默认 true，
      // 定时器到点也不会消失；显式关闭以恢复自动消失。
      persist: false,
      action: SnackBarAction(
        label: hhText(context, '关闭'),
        onPressed: () => messenger.hideCurrentSnackBar(),
      ),
    ),
  );
}

/// 头像选择与圆形裁切流程：
/// 相册弹窗（权限拒绝时回退系统选择器）→ 圆形裁切编辑界面 → 已校验的 PNG。
/// 返回 null 表示用户取消。
Future<Uint8List?> pickIdentityAvatarImage(BuildContext context) async {
  var source = await _pickSource(context);
  while (source != null) {
    if (!context.mounted) return null;
    final bytes = source.bytes;
    final result = await Navigator.of(context).push<Object>(
      MaterialPageRoute<Object>(
        builder: (_) => _AvatarCropEditor(bytes: bytes),
      ),
    );
    if (result is Uint8List) return result;
    if (result != _CropAction.chooseAnother || !context.mounted) return null;
    // 「重新选择」：回到相册（或系统选择器）再选一次。
    source = await _pickSource(context, preferSystem: source.fromSystem);
  }
  return null;
}

typedef _SourceResult = ({Uint8List bytes, bool fromSystem});

Future<_SourceResult?> _pickSource(
  BuildContext context, {
  bool preferSystem = false,
}) async {
  final picker = AlbumPhotoPicker(context, avatar: true);
  final result = await picker.pickWithSource(1, preferSystem: preferSystem);
  if (!context.mounted || result.files.isEmpty) return null;
  final file = result.files.single;
  // Read at most one byte over the limit so the editor can show its size error.
  const readCap = 5 * 1024 * 1024;
  final builder = BytesBuilder(copy: false);
  try {
    if (await file.length() <= readCap) {
      await for (final chunk in file.openRead()) {
        final remaining = readCap + 1 - builder.length;
        builder.add(
          chunk.length <= remaining ? chunk : chunk.sublist(0, remaining),
        );
        if (builder.length > readCap) break;
      }
      return (bytes: builder.takeBytes(), fromSystem: result.fromSystem);
    }
    return (bytes: Uint8List(readCap + 1), fromSystem: result.fromSystem);
  } finally {
    await file.release?.call();
  }
}

class _AvatarCropEditor extends StatefulWidget {
  const _AvatarCropEditor({required this.bytes});
  final Uint8List bytes;
  @override
  State<_AvatarCropEditor> createState() => _AvatarCropEditorState();
}

class _AvatarCropEditorState extends State<_AvatarCropEditor> {
  final _transform = TransformationController();
  ui.Image? _image;
  String? _error;
  bool _busy = false;
  double _stage = 0;

  static const _tooLarge = '照片超过 5MB，请选择更小的照片。';
  static const _animated = '头像不支持动图，请选择静态照片。';
  static const _undecodable = '这张照片暂时无法读取，请换一张重试。';

  @override
  void initState() {
    super.initState();
    _transform.addListener(_constrainTransform);
    _decode();
  }

  Rect _imageRect(ui.Image image) {
    final longest = math.max(image.width, image.height);
    final shortest = math.min(image.width, image.height);
    final fit = math.max(
      _stage / longest,
      _stage * 2 * _cropRadiusFraction / shortest,
    );
    return Rect.fromCenter(
      center: Offset(_stage / 2, _stage / 2),
      width: image.width * fit,
      height: image.height * fit,
    );
  }

  void _constrainTransform() {
    final image = _image;
    if (image == null || _stage <= 0) return;
    final rect = _imageRect(image);
    final matrix = _transform.value;
    final scale = matrix.getMaxScaleOnAxis();
    final radius = _stage * _cropRadiusFraction;
    final roomX = math.max(0.0, rect.width * scale / 2 - radius);
    final roomY = math.max(0.0, rect.height * scale / 2 - radius);
    final centered = _stage / 2 * (1 - scale);
    final x = matrix.storage[12]
        .clamp(centered - roomX, centered + roomX)
        .toDouble();
    final y = matrix.storage[13]
        .clamp(centered - roomY, centered + roomY)
        .toDouble();
    if (x == matrix.storage[12] && y == matrix.storage[13]) return;
    _transform.value = Matrix4.copy(matrix)
      ..storage[12] = x
      ..storage[13] = y;
  }

  Future<void> _decode() async {
    setState(() {
      _error = null;
      _busy = true;
    });
    final bytes = widget.bytes;
    if (bytes.length > 5 * 1024 * 1024) {
      if (mounted) setState(() => _busy = false);
      await _decodeForPreview();
      if (mounted) setState(() => _error = _tooLarge);
      return;
    }
    try {
      final codec = await ui.instantiateImageCodec(bytes);
      try {
        if (codec.frameCount != 1) {
          await _decodeForPreview();
          if (mounted) setState(() => _error = _animated);
          return;
        }
        final frame = await codec.getNextFrame();
        if (mounted) {
          setState(() => _image = frame.image);
        } else {
          frame.image.dispose();
        }
      } finally {
        codec.dispose();
      }
    } catch (_) {
      if (mounted) setState(() => _error = _undecodable);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  // 违规图片也保持预览，方便用户对照重新选择。
  Future<void> _decodeForPreview() async {
    try {
      final codec = await ui.instantiateImageCodec(widget.bytes);
      try {
        final frame = await codec.getNextFrame();
        if (mounted) {
          setState(() => _image = frame.image);
        } else {
          frame.image.dispose();
        }
      } finally {
        codec.dispose();
      }
    } catch (_) {
      // 预览解码失败时只显示错误文案。
    }
  }

  @override
  void dispose() {
    _image?.dispose();
    _transform.dispose();
    super.dispose();
  }

  Future<void> _confirm() async {
    final image = _image;
    if (image == null || _stage <= 0 || _error != null || _busy) return;
    _constrainTransform();
    setState(() => _busy = true);
    try {
      final matrix = _transform.value;
      final scale = matrix.getMaxScaleOnAxis();
      // InteractiveViewer 的矩阵为缩放+平移：反推裁切圆心在子组件坐标系的位置。
      final center = ui.Offset(
        (_stage / 2 - matrix.storage[12]) / scale,
        (_stage / 2 - matrix.storage[13]) / scale,
      );
      final childRadius = (_stage * _cropRadiusFraction) / scale;
      final rect = _imageRect(image);
      final pixel = image.width / rect.width;
      var radius = childRadius * pixel;
      radius = math.min(radius, math.min(image.width, image.height) / 2);
      final cx = ((center.dx - rect.left) * pixel).clamp(
        radius,
        image.width - radius,
      );
      final cy = ((center.dy - rect.top) * pixel).clamp(
        radius,
        image.height - radius,
      );
      final out = math.max(1, math.min(radius, 320).round());
      final recorder = ui.PictureRecorder();
      final canvas = ui.Canvas(recorder);
      final paint = ui.Paint()..filterQuality = ui.FilterQuality.high;
      canvas.clipPath(
        ui.Path()..addOval(
          ui.Rect.fromCircle(
            center: ui.Offset(out.toDouble(), out.toDouble()),
            radius: out.toDouble(),
          ),
        ),
      );
      canvas.drawImageRect(
        image,
        ui.Rect.fromCircle(
          center: ui.Offset(cx.toDouble(), cy.toDouble()),
          radius: radius,
        ),
        ui.Rect.fromCircle(
          center: ui.Offset(out.toDouble(), out.toDouble()),
          radius: out.toDouble(),
        ),
        paint,
      );
      final picture = recorder.endRecording();
      try {
        final output = await picture.toImage(out * 2, out * 2);
        try {
          final data = await output.toByteData(format: ui.ImageByteFormat.png);
          if (data == null) {
            throw const FormatException('Avatar encoding failed');
          }
          if (mounted) {
            Navigator.pop(
              context,
              data.buffer.asUint8List(data.offsetInBytes, data.lengthInBytes),
            );
          }
        } finally {
          output.dispose();
        }
      } finally {
        picture.dispose();
      }
    } catch (_) {
      if (mounted) {
        setState(() => _error = _undecodable);
      }
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final p = HhTelegramPalette.of(context);
    final image = _image;
    return Scaffold(
      backgroundColor: p.background,
      appBar: AppBar(
        backgroundColor: p.surface,
        foregroundColor: p.ink,
        surfaceTintColor: Colors.transparent,
        elevation: 0,
        title: Text(
          hhText(context, '设置头像'),
          style: TextStyle(
            fontSize: 20,
            fontWeight: FontWeight.w700,
            color: p.ink,
          ),
        ),
      ),
      body: SafeArea(
        child: Column(
          children: [
            Expanded(
              child: Center(
                child: LayoutBuilder(
                  builder: (context, constraints) {
                    final stage = math.max(
                      0.0,
                      math.min(
                        constraints.biggest.width - 40,
                        constraints.biggest.height - 40,
                      ),
                    );
                    if (_stage != stage) {
                      _stage = stage;
                      WidgetsBinding.instance.addPostFrameCallback((_) {
                        if (mounted) _constrainTransform();
                      });
                    }
                    return SizedBox(
                      width: stage,
                      height: stage,
                      child: Stack(
                        children: [
                          if (image != null && stage > 0)
                            InteractiveViewer(
                              transformationController: _transform,
                              maxScale: 4,
                              minScale: 1,
                              boundaryMargin: const EdgeInsets.all(1e6),
                              child: SizedBox(
                                width: stage,
                                height: stage,
                                child: Stack(
                                  clipBehavior: Clip.none,
                                  children: [
                                    Positioned.fromRect(
                                      rect: _imageRect(image),
                                      child: RawImage(
                                        image: image,
                                        fit: BoxFit.fill,
                                      ),
                                    ),
                                  ],
                                ),
                              ),
                            ),
                          IgnorePointer(
                            child: CustomPaint(
                              painter: _CropRingPainter(
                                color: p.background,
                                ring: p.accent,
                              ),
                              size: Size.square(stage),
                            ),
                          ),
                        ],
                      ),
                    );
                  },
                ),
              ),
            ),
            Padding(
              padding: const EdgeInsets.only(bottom: 8),
              child: Text(
                hhText(context, '拖动调整位置 · 双指缩放'),
                style: TextStyle(fontSize: 12, color: p.secondary),
              ),
            ),
            if (_error case final error?)
              Padding(
                padding: const EdgeInsets.fromLTRB(24, 0, 24, 8),
                child: Text(
                  hhText(context, error),
                  textAlign: TextAlign.center,
                  style: TextStyle(fontSize: 13, color: p.error),
                ),
              ),
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 4, 20, 18),
              child: Row(
                children: [
                  Expanded(
                    child: OutlinedButton(
                      onPressed: () =>
                          Navigator.pop(context, _CropAction.chooseAnother),
                      child: Text(hhText(context, '重新选择')),
                    ),
                  ),
                  const SizedBox(width: 12),
                  Expanded(
                    child: FilledButton(
                      key: const ValueKey('avatar-crop-confirm'),
                      onPressed: _error != null || _busy
                          ? null
                          : () => _confirm(),
                      child: _busy
                          ? const SizedBox(
                              width: 16,
                              height: 16,
                              child: CircularProgressIndicator(strokeWidth: 2),
                            )
                          : Text(hhText(context, '确认')),
                    ),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _CropRingPainter extends CustomPainter {
  const _CropRingPainter({required this.color, required this.ring});
  final Color color, ring;
  @override
  void paint(Canvas canvas, Size size) {
    final radius = size.width * _cropRadiusFraction;
    final rect = Offset.zero & size;
    // 半透明遮罩盖住圆外区域，圆内保持透亮。
    canvas.saveLayer(rect.inflate(1), Paint());
    canvas.drawRect(rect, Paint()..color = color.withValues(alpha: .72));
    canvas.drawCircle(
      rect.center,
      radius,
      Paint()
        ..blendMode = BlendMode.clear
        ..color = const Color(0x00000000),
    );
    canvas.restore();
    canvas.drawCircle(
      rect.center,
      radius,
      Paint()
        ..style = PaintingStyle.stroke
        ..strokeWidth = 2
        ..color = ring,
    );
  }

  @override
  bool shouldRepaint(_CropRingPainter old) =>
      old.color != color || old.ring != ring;
}
