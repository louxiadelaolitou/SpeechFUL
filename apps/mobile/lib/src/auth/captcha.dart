import 'dart:typed_data';

import 'auth_models.dart';

class CaptchaChallenge {
  const CaptchaChallenge({
    required this.id,
    required this.background,
    required this.piece,
    required this.width,
    required this.height,
    required this.pieceWidth,
    required this.pieceHeight,
    required this.startX,
    required this.y,
    required this.expiresAt,
  });
  final String id;
  final Uint8List background, piece;
  final int width, height, pieceWidth, pieceHeight, startX, y;
  final DateTime expiresAt;
}

class CaptchaExchange {
  const CaptchaExchange({required this.load, required this.solve});
  final Future<CaptchaChallenge> Function() load;
  final Future<String> Function(String id, double x) solve;
}

typedef CaptchaPresenter = Future<String?> Function(CaptchaExchange exchange);

abstract interface class CaptchaApi {
  CaptchaPresenter? get captchaPresenter;
  set captchaPresenter(CaptchaPresenter? value);
}

/// The protected business POST has not been opened. Unlike network uncertainty
/// after that POST, this permits dropping a newly prepared local mail request.
class CaptchaNotSubmitted extends AuthFailure {
  const CaptchaNotSubmitted({this.cancelled = false})
    : super(kind: AuthFailureKind.rejected, code: 'CAPTCHA_NOT_COMPLETED');
  final bool cancelled;
}
