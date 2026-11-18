import 'dart:convert';

import '../auth/auth_crypto.dart';
import '../auth/auth_store.dart';

/// Reading state belongs to this installation, not to an account or session.
class StudentLetterStore {
  StudentLetterStore(this._vault);

  factory StudentLetterStore.native({
    required String environment,
  }) => StudentLetterStore(
    FlutterAuthVault(
      namespace:
          'hnuhole.student-letter.v1.${AuthCrypto.domainDigest('HNUHOLE/STUDENT-LETTER/V1', utf8.encode(jsonEncode(environment)))}',
    ),
  );

  final AuthVault _vault;
  static const _receipt = 'student-letter-v1:read';

  Future<bool> hasRead() async => await _vault.read() == _receipt;

  Future<void> markRead() async {
    await _vault.write(_receipt);
    if (await _vault.read() != _receipt) {
      throw const AuthStorageFailure();
    }
  }
}
