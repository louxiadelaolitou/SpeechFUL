import 'package:flutter_test/flutter_test.dart';
import 'package:hnuhole_mobile/hnuhole_mobile.dart';

import 'auth_flows_test.dart' as f;
import 'email_reset_test.dart' as reset;

class ExpiredApi extends reset.ResetApi {
  final keys = <String>[];
  @override
  Future<OtpRequestAccepted> requestEmailLoginOtp({
    required String email,
    required String installationId,
    required String idempotencyKey,
  }) async {
    keys.add(idempotencyKey);
    if (keys.length == 1) {
      throw const AuthFailure(kind: AuthFailureKind.timeout);
    }
    if (keys.length == 2) {
      throw const AuthFailure(
        kind: AuthFailureKind.rejected,
        statusCode: 410,
        code: 'RESULT_EXPIRED',
      );
    }
    return OtpRequestAccepted(flowId: f.bytes(32, 4), retryAfterSeconds: 60);
  }
}

void main() {
  for (final resetting in [false, true]) {
    test(
      'expired OTP can start a fresh send with no proof or C command reset=$resetting',
      () async {
        final api = ExpiredApi(), store = AuthStore(f.Vault());
        var clock = f.now;
        AuthFlows make() => AuthFlows(
          api: api,
          store: store,
          legacyRecoveryEnabled: false,
          clock: () => clock,
          clearCommunityAccess: () {},
          acceptRegistrationSession: (_) async {},
        );
        Future<void> send(AuthFlows flow) => resetting
            ? flow.requestEmailReset('owned@hainanu.edu.cn')
            : flow.requestEmailLogin('owned@hainanu.edu.cn');
        await send(make());
        clock = clock.add(const Duration(minutes: 11));
        await send(make());
        await send(make());
        expect(api.keys.length, 3);
        expect(api.keys[0], api.keys[1]);
        expect(api.keys[2], isNot(api.keys[1]));
        final saved = resetting
            ? store.current!.emailReset
            : store.current!.emailLogin;
        expect(saved!['flowId'], f.bytes(32, 4));
      },
    );
  }
}
